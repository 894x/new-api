package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const (
	routingMonitorRegistry       = "new-api:routing-monitor:v1:nodes"
	routingMonitorNodePrefix     = "new-api:routing-monitor:v1:node:"
	routingMonitorReportInterval = 3 * time.Second
	routingMonitorNodeTTL        = 15 * time.Second
	routingMonitorMaxNodes       = 16
	routingMonitorMaxNodeBytes   = 1 << 20
	routingMonitorMaxReplyBytes  = 4 << 20
)

type RoutingMonitorNode struct {
	ID        string                         `json:"id"`
	Name      string                         `json:"name"`
	UpdatedAt int64                          `json:"updated_at"`
	Local     bool                           `json:"local"`
	Truncated bool                           `json:"truncated"`
	Routes    []dynamicrouting.RouteSnapshot `json:"routes"`
}

type RoutingMonitorResponse struct {
	GeneratedAt   int64                `json:"generated_at"`
	Coverage      string               `json:"coverage"`
	LegacyEnabled bool                 `json:"legacy_enabled"`
	Scope         string               `json:"scope"`
	UserID        int                  `json:"user_id,omitempty"`
	Partial       bool                 `json:"partial"`
	Nodes         []RoutingMonitorNode `json:"nodes"`
}

var routingMonitorInstanceID = uuid.NewString()
var routingMonitorReporterOnce sync.Once

// Priority observations never enter the adaptive routing controller.
var priorityRoutingRuntime = struct {
	sync.Mutex
	controller *dynamicrouting.Controller
}{}

const routingMonitorDispatchContextKey = "routing_monitor_dispatch"

type routingMonitorDispatch struct {
	channelID int
	started   func(time.Time)
	finished  func(dynamicrouting.Sample)
}

func PriorityRoutingMonitorEnabled(c *gin.Context) bool {
	value, _ := c.Get(routingMonitorDispatchContextKey)
	dispatch, ok := value.(routingMonitorDispatch)
	return ok && dispatch.finished != nil
}

func ObservePriorityRoutingSample(c *gin.Context, sample dynamicrouting.Sample) {
	value, _ := c.Get("priority_routing_outcome")
	if observe, ok := value.(func(dynamicrouting.Sample)); ok {
		observe(sample)
	}
}

func recordPriorityRoutingSelection(param *RetryParam, group string, channel *model.Channel) {
	// Retry exclusions belong to one attempt, not to the pool's topology.
	monitorParam := *param
	monitorParam.AttemptedChannelIds = nil
	filters, err := monitorParam.SelectionFilters(group)
	if err != nil {
		return
	}
	eligible, err := model.ListChannelSelectionCandidates(group, param.ModelName, filters)
	if err != nil {
		return
	}
	candidates := make([]dynamicrouting.Candidate, 0, len(eligible))
	for _, entry := range eligible {
		priority := int64(0)
		if entry.Priority != nil {
			priority = *entry.Priority
		}
		candidates = append(candidates, dynamicrouting.Candidate{ChannelID: entry.ChannelId, Priority: priority, Weight: entry.Weight})
	}
	key := dynamicrouting.RouteKey{Group: group, Model: param.ModelName}
	strategy, err := param.RoutingStrategy(group)
	if err != nil {
		return
	}
	if len(filters.UserRoutingOverrides) > 0 || strategy != nil && strategy.UserId > 0 {
		key.Policy = fmt.Sprintf("user:%d", common.GetContextKeyInt(param.Ctx, constant.ContextKeyUserId))
	}
	priorityRoutingRuntime.Lock()
	if priorityRoutingRuntime.controller == nil {
		config := dynamic_routing_setting.GetSnapshot().Config
		config.Enabled = false
		priorityRoutingRuntime.controller, err = dynamicrouting.NewController(config)
	}
	controller := priorityRoutingRuntime.controller
	priorityRoutingRuntime.Unlock()
	if err != nil || controller == nil {
		return
	}
	startRoutingMonitorReporter()
	param.Ctx.Set(routingMonitorDispatchContextKey, routingMonitorDispatch{
		channelID: channel.Id,
		started:   func(at time.Time) { controller.RecordPriorityDispatch(key, candidates, channel.Id, at) },
		finished:  func(sample dynamicrouting.Sample) { controller.RecordPriorityOutcome(key, channel.Id, sample) },
	})
}

func recordPriorityChannelStatus(channelID int) {
	priorityRoutingRuntime.Lock()
	controller := priorityRoutingRuntime.controller
	priorityRoutingRuntime.Unlock()
	if controller == nil {
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return
	}
	reason, _ := channel.GetOtherInfo()["status_reason"].(string)
	if len(reason) > 1024 {
		reason = strings.ToValidUTF8(reason[:1024], "") + "…"
	}
	controller.RecordChannelStatus(channelID, channel.Status, reason, time.Now())
}

// BindRoutingMonitorAttempt consumes the selection's immutable observer. Only
// physical dispatch can invoke it; local rejection and retry selection cannot.
func BindRoutingMonitorAttempt(c *gin.Context, info *relaycommon.RelayInfo, channelID int) {
	c.Set("priority_routing_outcome", nil)
	observer, _ := c.Get(routingMonitorDispatchContextKey)
	c.Set(routingMonitorDispatchContextKey, nil)
	if dispatch, ok := observer.(routingMonitorDispatch); ok && dispatch.channelID == channelID {
		info.OnDynamicRoutingUpstreamStarted(dispatch.started)
		c.Set("priority_routing_outcome", dispatch.finished)
	}
}

var routingMonitorRemoteCache = struct {
	sync.Mutex
	client    *redis.Client
	readAt    time.Time
	nodes     []RoutingMonitorNode
	partial   bool
	available bool
}{}

// startRoutingMonitorReporter binds the client once, so shutdown or test-time
// replacement of the shared client cannot redirect a running publisher.
func startRoutingMonitorReporter() {
	client := common.RDB
	if !common.RedisEnabled || client == nil {
		return
	}
	routingMonitorReporterOnce.Do(func() {
		gopool.Go(func() {
			ticker := time.NewTicker(routingMonitorReportInterval)
			defer ticker.Stop()
			for {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				err := publishRoutingMonitorNode(ctx, client, localRoutingMonitorNode(time.Now()))
				cancel()
				if errors.Is(err, redis.ErrClosed) {
					return
				}
				<-ticker.C
			}
		})
	})
}

func localRoutingMonitorNode(now time.Time) RoutingMonitorNode {
	dynamicRoutingRuntime.Lock()
	controller := dynamicRoutingRuntime.controller
	dynamicRoutingRuntime.Unlock()
	node := RoutingMonitorNode{
		ID: routingMonitorInstanceID, Name: common.GetNodeIdentity().Name,
		UpdatedAt: now.UnixMilli(), Local: true, Routes: []dynamicrouting.RouteSnapshot{},
	}
	if node.Name == "" {
		node.Name = "gateway"
	}
	if len(node.Name) > 256 {
		name := []rune(node.Name)
		node.Name = string(name[:min(len(name), 64)])
	}
	if controller != nil {
		node.Routes = controller.MonitorSnapshot(now)
		node.Truncated = len(node.Routes) >= 512
	}
	priorityRoutingRuntime.Lock()
	priority := priorityRoutingRuntime.controller
	priorityRoutingRuntime.Unlock()
	if priority != nil {
		node.Routes = append(node.Routes, priority.MonitorSnapshot(now)...)
		if len(node.Routes) > 512 {
			node.Routes = node.Routes[:512]
			node.Truncated = true
		}
	}
	return node
}

// boundedRoutingMonitorNode keeps the freshest routes if a reporting node has
// more telemetry than the transport budget. Truncation is always visible.
func boundedRoutingMonitorNode(node RoutingMonitorNode) (RoutingMonitorNode, []byte, error) {
	slices.SortStableFunc(node.Routes, func(a, b dynamicrouting.RouteSnapshot) int {
		if a.LastSeen > b.LastSeen {
			return -1
		}
		if a.LastSeen < b.LastSeen {
			return 1
		}
		return 0
	})
	for {
		data, err := common.Marshal(node)
		if err != nil || len(data) <= routingMonitorMaxNodeBytes {
			return node, data, err
		}
		if len(node.Routes) == 0 {
			return node, nil, errors.New("routing monitor node exceeds transport limit")
		}
		node.Truncated = true
		node.Routes = node.Routes[:len(node.Routes)/2]
	}
}

func publishRoutingMonitorNode(ctx context.Context, client *redis.Client, node RoutingMonitorNode) error {
	node.Local = false
	node, payload, err := boundedRoutingMonitorNode(node)
	if err != nil {
		return err
	}
	key := routingMonitorNodePrefix + node.ID
	pipe := client.TxPipeline()
	pipe.Set(ctx, key, payload, routingMonitorNodeTTL)
	pipe.ZAdd(ctx, routingMonitorRegistry, &redis.Z{Score: float64(node.UpdatedAt), Member: key})
	pipe.ZRemRangeByScore(ctx, routingMonitorRegistry, "-inf", strconv.FormatInt(node.UpdatedAt-routingMonitorNodeTTL.Milliseconds(), 10))
	pipe.Expire(ctx, routingMonitorRegistry, 2*routingMonitorNodeTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func routingMonitorRemoteNodes(ctx context.Context, client *redis.Client, now time.Time) ([]RoutingMonitorNode, bool, bool) {
	routingMonitorRemoteCache.Lock()
	defer routingMonitorRemoteCache.Unlock()
	cache := &routingMonitorRemoteCache
	if cache.client == client && !cache.readAt.IsZero() && now.Sub(cache.readAt) >= 0 && now.Sub(cache.readAt) < 2*time.Second {
		return cache.nodes, cache.partial, cache.available
	}
	cache.client, cache.readAt = client, now
	cache.nodes, cache.partial, cache.available = nil, false, false
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	keys, err := client.ZRevRangeByScore(ctx, routingMonitorRegistry, &redis.ZRangeBy{
		Min: strconv.FormatInt(now.Add(-routingMonitorNodeTTL).UnixMilli(), 10),
		Max: "+inf", Count: routingMonitorMaxNodes + 1,
	}).Result()
	if err != nil {
		cache.partial = true
		return nil, true, false
	}
	cache.available = true
	cache.partial = len(keys) > routingMonitorMaxNodes
	if len(keys) > routingMonitorMaxNodes {
		keys = keys[:routingMonitorMaxNodes]
	}
	pipe := client.Pipeline()
	commands := make(map[string]*redis.StringCmd, len(keys))
	for _, key := range keys {
		if !strings.HasPrefix(key, routingMonitorNodePrefix) || key == routingMonitorNodePrefix+routingMonitorInstanceID {
			continue
		}
		// GETRANGE bounds bytes transferred even when a malformed record is huge.
		commands[key] = pipe.GetRange(ctx, key, 0, routingMonitorMaxNodeBytes)
	}
	if len(commands) == 0 {
		return nil, cache.partial, true
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		cache.partial = true
	}
	for key, command := range commands {
		payload, err := command.Result()
		if errors.Is(err, redis.Nil) || payload == "" {
			continue
		}
		if err != nil || len(payload) > routingMonitorMaxNodeBytes {
			cache.partial = true
			continue
		}
		var node RoutingMonitorNode
		if common.UnmarshalJsonStr(payload, &node) != nil || node.ID == "" || key != routingMonitorNodePrefix+node.ID || len(node.Name) > 256 || len(node.Routes) > 512 {
			cache.partial = true
			continue
		}
		if node.UpdatedAt < now.Add(-routingMonitorNodeTTL).UnixMilli() || node.UpdatedAt > now.Add(5*time.Second).UnixMilli() {
			continue
		}
		node.Local = false
		node.Truncated = node.Truncated || len(node.Routes) >= 512
		cache.nodes = append(cache.nodes, node)
	}
	return cache.nodes, cache.partial, cache.available
}

// GetRoutingMonitor returns detached observations; it does not create a routing
// controller, select a channel, advance recovery, or query the primary database.
// User scope includes its inherited base pool, never another user's policy.
func GetRoutingMonitor(ctx context.Context, userID int) RoutingMonitorResponse {
	now := time.Now()
	response := RoutingMonitorResponse{GeneratedAt: now.UnixMilli(), Coverage: "local", LegacyEnabled: DynamicRoutingEnabled(), Scope: "base", Nodes: []RoutingMonitorNode{}}
	if userID > 0 {
		response.Scope, response.UserID = "user", userID
	}
	nodes := []RoutingMonitorNode{localRoutingMonitorNode(now)}
	if common.RedisEnabled && common.RDB != nil {
		remote, partial, available := routingMonitorRemoteNodes(ctx, common.RDB, now)
		response.Partial = partial
		if available {
			response.Coverage = "cluster"
		}
		nodes = append(nodes, remote...)
	}
	policy := ""
	if userID > 0 {
		policy = fmt.Sprintf("user:%d", userID)
	}
	remaining := routingMonitorMaxReplyBytes
	for _, node := range nodes {
		if len(response.Nodes) >= routingMonitorMaxNodes {
			response.Partial = true
			break
		}
		if !node.Local && now.UnixMilli()-node.UpdatedAt > routingMonitorNodeTTL.Milliseconds() {
			continue
		}
		filtered := make([]dynamicrouting.RouteSnapshot, 0, len(node.Routes))
		for _, route := range node.Routes {
			if route.Policy == "" || userID > 0 && route.Policy == policy {
				filtered = append(filtered, route)
				response.Partial = response.Partial || route.Truncated
			}
		}
		node.Routes = filtered
		bounded, data, err := boundedRoutingMonitorNode(node)
		if err != nil || len(data) > remaining {
			response.Partial = true
			continue
		}
		remaining -= len(data)
		response.Partial = response.Partial || bounded.Truncated
		response.Nodes = append(response.Nodes, bounded)
	}
	slices.SortStableFunc(response.Nodes, func(a, b RoutingMonitorNode) int {
		if a.Local != b.Local {
			if a.Local {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return response
}
