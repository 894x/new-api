package relay

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type TaskSubmitResult struct {
	UpstreamTaskID string
	TaskData       []byte
	ClientResponse any
	Platform       constant.TaskPlatform
	Quota          int // fixed-group fallback quota; controller replaces it with the settled net amount
	OriginalQuota  int // true pre-group amount used by monthly group/model tier calculation
	responseStatus int
	responseHeader http.Header
	responseBody   []byte
	Immediate      *relaycommon.TaskInfo
	PluginState    []byte
	//PerCallPrice   types.PriceData
}

// WriteResponse publishes the upstream success response only after the task's
// durable billing state has been inserted and finalized by the controller.
func (r *TaskSubmitResult) WriteResponse(c *gin.Context) error {
	if r == nil || c == nil || c.Writer == nil {
		return errors.New("task response target is nil")
	}
	for key, values := range r.responseHeader {
		c.Writer.Header()[key] = append([]string(nil), values...)
	}
	status := r.responseStatus
	if status == 0 {
		status = http.StatusOK
	}
	c.Writer.WriteHeader(status)
	if len(r.responseBody) == 0 {
		c.Writer.WriteHeaderNow()
		return nil
	}
	_, err := c.Writer.Write(r.responseBody)
	return err
}

type taskBufferedResponseWriter struct {
	original gin.ResponseWriter
	header   http.Header
	body     bytes.Buffer
	status   int
	size     int
}

func newTaskBufferedResponseWriter(original gin.ResponseWriter) *taskBufferedResponseWriter {
	return &taskBufferedResponseWriter{
		original: original,
		header:   original.Header().Clone(),
		status:   http.StatusOK,
		size:     -1,
	}
}

func (w *taskBufferedResponseWriter) Header() http.Header { return w.header }

func (w *taskBufferedResponseWriter) WriteHeader(code int) {
	if code > 0 && !w.Written() {
		w.status = code
	}
}

func (w *taskBufferedResponseWriter) WriteHeaderNow() {
	if !w.Written() {
		w.size = 0
	}
}

func (w *taskBufferedResponseWriter) Write(data []byte) (int, error) {
	w.WriteHeaderNow()
	n, err := w.body.Write(data)
	w.size += n
	return n, err
}

func (w *taskBufferedResponseWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}

func (w *taskBufferedResponseWriter) Status() int   { return w.status }
func (w *taskBufferedResponseWriter) Size() int     { return w.size }
func (w *taskBufferedResponseWriter) Written() bool { return w.size >= 0 }
func (w *taskBufferedResponseWriter) Flush()        { w.WriteHeaderNow() }
func (w *taskBufferedResponseWriter) CloseNotify() <-chan bool {
	return w.original.CloseNotify()
}
func (w *taskBufferedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.original.Hijack()
}
func (w *taskBufferedResponseWriter) Pusher() http.Pusher { return w.original.Pusher() }

// ResolveOriginTask 处理基于已有任务的提交（remix / continuation）：
// 查找原始任务、从中提取模型名称、将渠道锁定到原始任务的渠道
// （通过 info.LockedChannel，重试时复用同一渠道并轮换 key），
// 以及提取 OtherRatios（时长、分辨率）。
// 该函数在控制器的重试循环之前调用一次，其结果通过 info 字段和上下文持久化。
func ResolveOriginTask(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	// 检测 remix action
	path := c.Request.URL.Path
	if strings.Contains(path, "/v1/videos/") && strings.HasSuffix(path, "/remix") {
		info.Action = constant.TaskActionRemix
	}

	// 提取 remix 任务的 video_id
	if info.Action == constant.TaskActionRemix {
		videoID := c.Param("video_id")
		if strings.TrimSpace(videoID) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video_id is required"), "invalid_request", http.StatusBadRequest)
		}
		info.OriginTaskID = videoID
	}

	if info.OriginTaskID == "" {
		return nil
	}

	// 查找原始任务
	originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
	}
	if !exist {
		return service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
	}

	// 从原始任务推导模型名称
	restoredOriginModel := false
	if info.OriginModelName == "" {
		if originTask.Properties.OriginModelName != "" {
			info.OriginModelName = originTask.Properties.OriginModelName
		} else if originTask.Properties.UpstreamModelName != "" {
			info.OriginModelName = originTask.Properties.UpstreamModelName
		} else {
			var taskData map[string]any
			_ = common.Unmarshal(originTask.Data, &taskData)
			if m, ok := taskData["model"].(string); ok && m != "" {
				info.OriginModelName = m
			}
		}
		restoredOriginModel = info.OriginModelName != ""
	}
	if restoredOriginModel {
		info.BindGroupModelDiscountResolver()
	}

	// 锁定到原始任务的渠道（重试时复用同一渠道，轮换 key）
	ch, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
	}
	if ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task is disabled"), "task_channel_disable", http.StatusBadRequest)
	}
	info.LockedChannel = ch

	if originTask.ChannelId != info.ChannelId {
		key, _, newAPIError := ch.GetNextEnabledKey()
		if newAPIError != nil {
			return service.TaskErrorWrapper(newAPIError, "channel_no_available_key", newAPIError.StatusCode)
		}
		common.SetContextKey(c, constant.ContextKeyChannelKey, key)
		common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
		common.SetContextKey(c, constant.ContextKeyChannelId, originTask.ChannelId)

		info.ChannelBaseUrl = ch.GetBaseURL()
		info.ChannelId = originTask.ChannelId
		info.ChannelType = ch.Type
		info.ApiKey = key
	}

	// 提取 remix 参数（时长、分辨率 → OtherRatios）
	if info.Action == constant.TaskActionRemix {
		if originTask.PrivateData.BillingContext != nil {
			// 新的 remix 逻辑：直接从原始任务的 BillingContext 中提取 OtherRatios（如果存在）
			for s, f := range originTask.PrivateData.BillingContext.OtherRatios {
				info.PriceData.AddOtherRatio(s, f)
			}
		} else {
			// 旧的 remix 逻辑：直接从 task data 解析 seconds 和 size（如果存在）
			var taskData map[string]any
			_ = common.Unmarshal(originTask.Data, &taskData)
			secondsStr, _ := taskData["seconds"].(string)
			seconds, _ := strconv.Atoi(secondsStr)
			if seconds <= 0 {
				seconds = 4
			}
			// 历史任务数据可能包含未经校验的时长，作为计费乘数前必须钳制
			if seconds > relaycommon.MaxTaskDurationSeconds {
				seconds = relaycommon.MaxTaskDurationSeconds
			}
			sizeStr, _ := taskData["size"].(string)
			info.PriceData.AddOtherRatio("seconds", float64(seconds))
			info.PriceData.AddOtherRatio("size", 1)
			if sizeStr == "1792x1024" || sizeStr == "1024x1792" {
				info.PriceData.AddOtherRatio("size", 1.666667)
			}
		}
	}

	return nil
}

// ApplyChannelPin copies plugin-declared origin-task facts from the prepare
// context onto RelayInfo and, when the resolved pin retries on the same
// channel, writes LockedChannel. ResolveOriginTask is unchanged.
func ApplyChannelPin(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if info == nil {
		return nil
	}
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	if tasks, ok := common.GetContextKeyType[[]*model.Task](c, constant.ContextKeyOriginTasks); ok {
		refs := make([]relaycommon.OriginTaskRef, 0, len(tasks))
		for _, task := range tasks {
			if task == nil {
				continue
			}
			refs = append(refs, relaycommon.OriginTaskRef{
				TaskID:         task.TaskID,
				UpstreamTaskID: task.GetUpstreamTaskID(),
				Action:         task.Action,
				Status:         string(task.Status),
				Data:           append([]byte(nil), task.Data...),
			})
		}
		info.OriginTasks = refs
	}
	pin, found, _ := service.GetChannelConstraints(c).ResolvedPin()
	if !found || pin.RetryMode != dto.PinRetrySameChannel {
		return nil
	}
	ch, err := model.CacheGetChannel(pin.ChannelId)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "origin_task_channel_disabled", http.StatusBadRequest)
	}
	if ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task is disabled"), "origin_task_channel_disabled", http.StatusBadRequest)
	}
	info.LockedChannel = ch
	return nil
}

// ApplyOriginTaskAffinity is the compatibility name for ApplyChannelPin.
func ApplyOriginTaskAffinity(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	return ApplyChannelPin(c, info)
}

// RelayTaskSubmit 完成 task 提交的全部流程（每次尝试调用一次）：
// 刷新渠道元数据 → 确定 platform/adaptor → 验证请求 →
// 估算计费(EstimateBilling) → 计算价格 → 预扣费（仅首次）→
// 构建/发送/解析上游请求 → 提交后计费调整(AdjustBillingOnSubmit)。
// 共享控制器编排负责未落库退款、最终额度预留、落库和结算。
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo) (*TaskSubmitResult, *dto.TaskError) {
	info.InitChannelMeta(c)

	// 1. 确定 platform → 创建适配器 → 验证请求
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}
	platform, adaptor := getTaskAdaptorForRequest(c, platform)
	if adaptor == nil {
		code, message := TaskPlatformUnavailableError(platform)
		return nil, service.TaskErrorWrapperLocal(errors.New(message), code, http.StatusBadRequest)
	}
	// buildSubmitRequest runs during validation and the unreleased plugin
	// contract exposes this host-generated id to that hook.
	if info.PublicTaskID == "" {
		info.PublicTaskID = model.GenerateTaskID()
	}
	taskResponseFormat := common.GetContextKeyString(c, constant.ContextKeyTaskResponseFormat)
	if native, ok := adaptor.(channel.NativeTaskProtocol); ok && taskResponseFormat != "" {
		if !native.SupportsNativeTaskFormat(taskResponseFormat) {
			return nil, service.TaskErrorWrapperLocal(errors.New("selected channel does not support the requested native video protocol"), "invalid_api_platform", http.StatusBadRequest)
		}
	} else if taskResponseFormat == constant.TaskResponseFormatDoubaoVideo {
		if _, ok := adaptor.(channel.NativeVideoConverter); !ok {
			return nil, service.TaskErrorWrapperLocal(errors.New("selected channel does not support the Doubao video protocol"), "invalid_api_platform", http.StatusBadRequest)
		}
	} else if taskResponseFormat == constant.TaskResponseFormatAliVideo {
		if _, ok := adaptor.(channel.AliNativeVideoConverter); !ok {
			return nil, service.TaskErrorWrapperLocal(errors.New("selected channel does not support the Ali video protocol"), "invalid_api_platform", http.StatusBadRequest)
		}
	}
	if _, pluginNative := adaptor.(channel.NativeTaskProtocol); !pluginNative && taskResponseFormat == constant.TaskResponseFormatMiniMaxVideoV2 {
		if _, ok := adaptor.(channel.MiniMaxVideoV2Converter); !ok {
			return nil, service.TaskErrorWrapperLocal(errors.New("selected channel does not support the MiniMax video V2 protocol"), "invalid_api_platform", http.StatusBadRequest)
		}
	}
	adaptor.Init(info)
	// Submit hooks cache the upstream body during validation. Resolve the
	// channel mapping first, but keep the public model as the billing identity.
	mappedBeforeValidate := info.OriginModelName != ""
	if mappedBeforeValidate {
		info.UpstreamModelName = info.OriginModelName
		if err := helper.ModelMappedHelper(c, info, nil); err != nil {
			return nil, service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest)
		}
	}
	// Capture the client contract before asset rewriting and channel policies.
	// Persist only expression-referenced probes, not this full transient input.
	// Mapped aliases may inherit a target expression after submit-hook model
	// rewriting; their original request must be available at that point too.
	if info.BillingRequestInput == nil && (info.IsModelMapped || billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeTieredExpr) {
		input, inputErr := helper.ResolveIncomingBillingExprRequestInput(c, info)
		if inputErr != nil {
			return nil, service.TaskErrorWrapperLocal(inputErr, "model_price_error", http.StatusBadRequest)
		}
		if taskResponseFormat != "" {
			if native := gjson.GetBytes(input.Body, "metadata"); native.IsObject() {
				input.Body = []byte(native.Raw)
			}
		}
		info.BillingRequestInput = &input
	}
	if platform != constant.TaskPlatformSuno {
		if err := prepareManagedVideoRequest(c, info.UserId); err != nil {
			return nil, service.TaskErrorWrapperLocal(err, "asset_storage_failed", http.StatusBadRequest)
		}
	}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return nil, taskErr
	}

	// 2. 确定模型名称
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
	}

	if !mappedBeforeValidate {
		info.OriginModelName = modelName
		info.UpstreamModelName = modelName
		if err := helper.ModelMappedHelper(c, info, nil); err != nil {
			return nil, service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest)
		}
	}
	if validator, ok := adaptor.(channel.MappedTaskRequestValidator); ok {
		if taskErr := validator.ValidateMappedRequest(c, info); taskErr != nil {
			return nil, taskErr
		}
	}

	// 4. 价格计算：基础模型价格
	info.OriginModelName = modelName
	var priceData types.PriceData
	var err error
	pluginKey := c.GetString("task_plugin_key")
	pinnedValue, _ := c.Get(jsplugin.ContextKeyPinnedPlugin)
	pinnedPlugin, _ := pinnedValue.(jsplugin.PinnedPlugin)
	if pinnedPlugin.Plugin != nil {
		pluginKey = pinnedPlugin.Plugin.Meta.Key
	}
	exprStr, exists := billing_setting.ResolveTaskBillingExpr(pluginKey, modelName, info.UpstreamModelName)
	useTiered := exists || billing_setting.GetBillingMode(modelName) == billing_setting.BillingModeTieredExpr
	frozenBilling := info.TieredBillingSnapshot
	reuseTaskExpression := frozenBilling != nil && frozenBilling.TaskUsageBilling && frozenBilling.ModelName == modelName && (frozenBilling.TaskPluginKey == "" || frozenBilling.TaskPluginKey == pluginKey)
	if reuseTaskExpression || useTiered {
		quotaPerUnit := common.QuotaPerUnit
		var frozenRequest *billingexpr.RequestInput
		if reuseTaskExpression {
			exprStr, exists = frozenBilling.ExprString, true
			quotaPerUnit = frozenBilling.QuotaPerUnit
			frozenRequest = frozenBilling.RequestInput
		}
		provider, supported := adaptor.(channel.TaskUsageFactsProvider)
		if billingexpr.UsesFixedPricing(exprStr) {
			return nil, service.TaskErrorWrapper(fmt.Errorf("fixed pricing is not supported for task usage expressions"), "model_price_error", http.StatusBadRequest)
		}
		if !exists || !supported {
			return nil, service.TaskErrorWrapper(fmt.Errorf("task model %s has no usage expression or meter", modelName), "model_price_error", http.StatusBadRequest)
		}
		sharedModel := pinnedPlugin.Generation.SharedModel(modelName) || pinnedPlugin.Generation.SharedModel(info.UpstreamModelName)
		if sharedModel && pinnedPlugin.Plugin != nil {
			schema, _ := pinnedPlugin.Plugin.Meta.UsageForModels(info.UpstreamModelName, modelName)
			if !billing_setting.TaskExprCompatible(exprStr, schema) {
				return nil, service.TaskErrorWrapper(fmt.Errorf("task model %s pricing is not configured for plugin %s", modelName, pluginKey), "model_price_error", http.StatusBadRequest)
			}
		}
		var facts map[string]any
		if validatedProvider, ok := adaptor.(channel.TaskValidatedUsageFactsProvider); ok {
			facts, err = validatedProvider.ExtractUsageFactsValidated(c, info)
			if err != nil {
				return nil, service.TaskErrorWrapperLocal(err, "plugin_usage_invalid", http.StatusBadRequest)
			}
		} else {
			facts = provider.ExtractUsageFacts(c, info)
		}
		input := billingexpr.RequestInput{}
		if info.BillingRequestInput != nil {
			input = *info.BillingRequestInput
		}
		if frozenRequest == nil {
			var snapshotErr error
			frozenRequest, snapshotErr = billingexpr.SnapshotRequestInput(exprStr, input, info.StartTime)
			if snapshotErr != nil {
				return nil, service.TaskErrorWrapperLocal(snapshotErr, "model_price_error", http.StatusBadRequest)
			}
		}
		evaluationInput := *frozenRequest
		evaluationInput.Usage = facts
		cost, trace, runErr := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{}, evaluationInput)
		if runErr != nil || cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
			if runErr == nil {
				runErr = fmt.Errorf("task expression result must be finite and non-negative")
			}
			return nil, service.TaskErrorWrapper(runErr, "model_price_error", http.StatusBadRequest)
		}
		groupRatioInfo := helper.HandleGroupRatio(c, info)
		snapshot, active, snapshotErr := info.ResolveGroupModelDiscount()
		if snapshotErr != nil {
			return nil, service.TaskErrorWrapperLocal(snapshotErr, "model_price_error", http.StatusBadRequest)
		}
		info.GroupModelDiscountSnapshot = nil
		if active {
			info.GroupModelDiscountSnapshot = &snapshot
		}
		originalQuota, originalClamp := common.QuotaRoundChecked(cost * quotaPerUnit)
		if active && originalClamp != nil {
			return nil, service.TaskErrorWrapperLocal(originalClamp, "model_price_error", http.StatusBadRequest)
		}
		if active && cost > 0 && originalQuota == 0 {
			originalQuota = 1
		}
		quota, clamp := common.QuotaRoundChecked(cost * quotaPerUnit * groupRatioInfo.GroupRatio)
		noteTaskQuotaClamp(info, clamp)
		priceData = types.PriceData{OriginalQuota: originalQuota, Quota: quota, QuotaToPreConsume: quota, GroupRatioInfo: groupRatioInfo}
		info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: billing_setting.BillingModeTieredExpr, ModelName: modelName, ExprString: exprStr, ExprHash: billingexpr.ExprHashString(exprStr), GroupRatio: groupRatioInfo.GroupRatio, EstimatedQuotaBeforeGroup: cost * quotaPerUnit, EstimatedQuotaAfterGroup: quota, EstimatedTier: trace.MatchedTier, QuotaPerUnit: quotaPerUnit, ExprVersion: billingexpr.ExprVersion(exprStr), TaskUsageBilling: true, UsageFacts: facts}
		info.TieredBillingSnapshot.RequestInput = frozenRequest
		info.TieredBillingSnapshot.TaskPluginKey = pluginKey
		info.TieredBillingSnapshot.RequestRules = trace.RequestRules
	} else {
		info.TieredBillingSnapshot = nil
		priceData, err = helper.ModelPriceHelperPerCall(c, info)
		if err != nil {
			return nil, service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest)
		}
	}
	info.PriceData = priceData

	// 5. 计费估算：让适配器根据用户请求提供 OtherRatios（时长、分辨率等）
	//    必须在 ModelPriceHelperPerCall 之后调用（它会重建 PriceData）。
	//    ResolveOriginTask 可能已在 remix 路径中预设了 OtherRatios，此处合并。
	if info.TieredBillingSnapshot == nil {
		var estimatedRatios map[string]float64
		if validatedProvider, ok := adaptor.(channel.TaskValidatedBillingProvider); ok {
			estimatedRatios, err = validatedProvider.EstimateBillingValidated(c, info)
			if err != nil {
				return nil, service.TaskErrorWrapperLocal(err, "plugin_usage_invalid", http.StatusBadRequest)
			}
		} else {
			estimatedRatios = adaptor.EstimateBilling(c, info)
		}
		if len(estimatedRatios) > 0 {
			for k, v := range estimatedRatios {
				info.PriceData.AddOtherRatio(k, v)
			}
		}
	}

	// 6. 将 OtherRatios 应用到基础额度（饱和转换，防止溢出成负数）
	if info.TieredBillingSnapshot == nil && !common.StringsContains(constant.TaskPricePatches, modelName) {
		if ratios := info.PriceData.OtherRatios(); len(ratios) > 0 {
			if _, ok := recalcQuotaFromRatios(info, ratios); !ok {
				return nil, service.TaskErrorWrapperLocal(errors.New("invalid task billing ratios"), "model_price_error", http.StatusBadRequest)
			}
		}
	}

	// 7. Reserve the safe amount for every routing attempt. A retry can switch
	// groups and reprice the task, so an existing billing session may need to be
	// raised before any request body is built or sent upstream.
	if taskErr := reserveTaskSubmitQuota(c, info); taskErr != nil {
		return nil, taskErr
	}

	// 8. 构建请求体
	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
	}

	// 9. 发送请求
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if resp == nil {
		return nil, service.TaskErrorWrapperLocal(errors.New("upstream returned an empty response"), "fail_to_fetch_task", http.StatusBadGateway)
	}
	defer resp.Body.Close()
	// Any 2xx reaches the plugin parser; preserve provider-specific error
	// decoding for all other statuses.
	if resp.StatusCode/100 != 2 {
		if parser, ok := adaptor.(channel.TaskSubmitErrorParser); ok {
			return nil, parser.ParseSubmitError(c, resp, info)
		}
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, service.TaskErrorWrapper(fmt.Errorf("%s", string(responseBody)), "fail_to_fetch_task", resp.StatusCode)
	}

	// 10. Parse only. The controller presents the response after the durable
	// task barrier and billing settlement.
	parsed, taskErr := adaptor.ParseResponse(c, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}
	if parsed == nil {
		return nil, service.TaskErrorWrapperLocal(errors.New("task adaptor returned an empty response"), "plugin_submit_response_invalid", http.StatusBadGateway)
	}

	// 11. 提交后计费调整：让适配器根据上游实际返回调整 OtherRatios
	finalQuota := info.PriceData.Quota
	if parsed.Immediate != nil && parsed.Immediate.Status == model.TaskStatusFailure {
		finalQuota = 0
	} else if info.TieredBillingSnapshot == nil {
		if adjustedRatios := adaptor.AdjustBillingOnSubmit(info, parsed.TaskData); len(adjustedRatios) > 0 {
			if adjustedQuota, ok := recalcQuotaFromRatios(info, adjustedRatios); ok {
				// 基于调整后的 ratios 重新计算 quota
				finalQuota = adjustedQuota
				info.PriceData.ReplaceOtherRatios(adjustedRatios)
				info.PriceData.Quota = finalQuota
			}
		}
	}

	if immediate := parsed.Immediate; immediate != nil {
		switch immediate.Status {
		case model.TaskStatusFailure:
			info.PriceData.OriginalQuota, info.PriceData.Quota = 0, 0
			finalQuota = 0
		case model.TaskStatusSuccess:
			finalQuota = priceImmediateTaskCompletion(c, adaptor, info, platform, parsed)
		}
	}
	info.PriceData.Quota = finalQuota
	return &TaskSubmitResult{
		UpstreamTaskID: parsed.UpstreamTaskID,
		TaskData:       parsed.TaskData,
		ClientResponse: parsed.ClientResponse,
		Platform:       platform,
		Quota:          finalQuota,
		OriginalQuota:  info.PriceData.OriginalQuota,
		Immediate:      parsed.Immediate,
		PluginState:    parsed.PluginState,
	}, nil
}

// priceImmediateTaskCompletion calculates the final price before the durable
// submission barrier. The controller then reserves and settles it once, using
// the same monthly-discount policy as asynchronously completed tasks.
func priceImmediateTaskCompletion(c *gin.Context, adaptor channel.TaskAdaptor, info *relaycommon.RelayInfo, platform constant.TaskPlatform, parsed *channel.TaskSubmitResponse) int {
	result := parsed.Immediate
	if snap := info.TieredBillingSnapshot; snap != nil {
		price, err := billingexpr.ComputeTaskUsageQuota(snap, result.UsageFacts)
		if err != nil {
			logger.LogWarn(c, "immediate task expression failed; retaining reservation: "+err.Error())
			return info.PriceData.Quota
		}
		original, clamp := common.QuotaRoundChecked(price.ActualQuotaBeforeGroup)
		if info.GroupModelDiscountSnapshot != nil && price.ActualQuotaBeforeGroup > 0 && original == 0 {
			original = 1
		}
		noteTaskQuotaClamp(info, clamp)
		noteTaskQuotaClamp(info, price.Clamp)
		info.PriceData.OriginalQuota, info.PriceData.Quota = original, price.ActualQuotaAfterGroup
		return info.PriceData.Quota
	}
	if info.PriceData.UsePrice || common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		return info.PriceData.Quota
	}
	task := model.InitTask(platform, info)
	task.Status, task.Data = model.TaskStatusSuccess, parsed.TaskData
	task.PrivateData.UpstreamTaskID = parsed.UpstreamTaskID
	task.PrivateData.Usage, task.Usage = result.Usage, result.Usage
	task.PrivateData.BillingContext = &model.TaskBillingContext{ModelRatio: info.PriceData.ModelRatio, GroupRatio: info.PriceData.GroupRatioInfo.GroupRatio, OtherRatios: info.PriceData.OtherRatios(), OriginModelName: info.OriginModelName}
	actualQuota := adaptor.AdjustBillingOnComplete(task, result)
	if actualQuota > 0 && info.GroupModelDiscountSnapshot == nil {
		info.PriceData.Quota = actualQuota
		return actualQuota
	}
	tokens := result.TotalTokens
	if tokens == 0 {
		tokens = result.CompletionTokens
	}
	if tokens <= 0 || info.PriceData.ModelRatio <= 0 {
		return info.PriceData.Quota
	}
	price := info.PriceData
	if !price.ReplaceOtherRatios(task.PrivateData.BillingContext.OtherRatios) {
		return info.PriceData.Quota
	}
	originalRaw := price.ApplyOtherRatiosToFloat(float64(tokens) * price.ModelRatio)
	original, originalClamp := common.QuotaFromFloatChecked(originalRaw)
	if originalRaw > 0 && original == 0 {
		original = 1
	}
	net, netClamp := common.QuotaFromFloatChecked(originalRaw * price.GroupRatioInfo.GroupRatio)
	noteTaskQuotaClamp(info, result.QuotaClamp)
	if info.GroupModelDiscountSnapshot != nil {
		noteTaskQuotaClamp(info, originalClamp)
	}
	noteTaskQuotaClamp(info, netClamp)
	price.OriginalQuota, price.Quota = original, net
	info.PriceData = price
	return net
}

func reserveTaskSubmitQuota(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if info == nil {
		return service.TaskErrorWrapperLocal(errors.New("relay info is nil"), "pre_consume_quota_failed", http.StatusInternalServerError)
	}
	targetQuota := taskQuotaToPreConsume(info)
	if info.Billing == nil {
		if info.PriceData.FreeModel && info.GroupModelDiscountSnapshot == nil {
			return nil
		}
		info.ForcePreConsume = true
		if apiErr := service.PreConsumeBilling(c, targetQuota, info); apiErr != nil {
			return service.TaskErrorFromAPIError(apiErr)
		}
		return nil
	}

	reserveErr := info.Billing.ReserveForAdmission(targetQuota)
	if reserveErr != nil {
		var apiErr *relaytypes.NewAPIError
		if errors.As(reserveErr, &apiErr) {
			taskErr := service.TaskErrorFromAPIError(apiErr)
			taskErr.LocalError = true
			return taskErr
		}
		return service.TaskErrorWrapperLocal(reserveErr, "pre_consume_quota_failed", http.StatusInternalServerError)
	}
	info.FinalPreConsumedQuota = info.Billing.GetPreConsumedQuota()
	return nil
}

func taskQuotaToPreConsume(info *relaycommon.RelayInfo) int {
	if info == nil {
		return 0
	}
	if info.GroupModelDiscountSnapshot != nil {
		return info.PriceData.OriginalQuota
	}
	return info.PriceData.Quota
}

// recalcQuotaFromRatios 根据 adjustedRatios 从冻结的模型价格输入重新计算
// 原价与结算价。不得通过已取整、已乘分组倍率的 quota 反向除倍率恢复原价。
func recalcQuotaFromRatios(info *relaycommon.RelayInfo, ratios map[string]float64) (int, bool) {
	priceData := info.PriceData
	if !priceData.ReplaceOtherRatios(ratios) {
		return 0, false
	}

	var originalBase float64
	if priceData.UsePrice {
		originalBase = priceData.ModelPrice * common.QuotaPerUnit
	} else {
		originalBase = priceData.ModelRatio / 2 * common.QuotaPerUnit
	}
	if originalBase < 0 {
		return 0, false
	}

	rawOriginalQuota := priceData.ApplyOtherRatiosToFloat(originalBase)
	originalQuota, originalClamp := common.QuotaFromFloatChecked(rawOriginalQuota)
	if info.GroupModelDiscountSnapshot != nil && rawOriginalQuota > 0 && originalQuota == 0 {
		originalQuota = 1
	}
	// Keep the legacy fixed-group rounding order for the fallback net amount:
	// group price is converted first, then request-specific multipliers apply.
	// Only the monthly ledger uses the independently calculated true original.
	netBaseQuota, netBaseClamp := common.QuotaFromFloatChecked(originalBase * priceData.GroupRatioInfo.GroupRatio)
	netQuota, netClamp := common.QuotaFromFloatChecked(
		priceData.ApplyOtherRatiosToFloat(float64(netBaseQuota)),
	)
	if info.GroupModelDiscountSnapshot != nil {
		noteTaskQuotaClamp(info, originalClamp)
	}
	noteTaskQuotaClamp(info, netBaseClamp)
	noteTaskQuotaClamp(info, netClamp)
	priceData.OriginalQuota = originalQuota
	priceData.Quota = netQuota
	info.PriceData = priceData
	return netQuota, true
}

// noteTaskQuotaClamp records the first quota saturation event onto the task's
// RelayInfo so LogTaskConsumption can surface it on the submit log's
// admin_info. First non-nil clamp wins.
func noteTaskQuotaClamp(info *relaycommon.RelayInfo, clamp *common.QuotaClamp) {
	if clamp == nil || info == nil {
		return
	}
	if info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
}

var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,
}

func RelayTaskFetch(c *gin.Context, relayMode int) (taskResp *dto.TaskError) {
	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		taskResp = service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
	}

	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}
	respBody = service.TaskResponseDataForClient(c, respBody)

	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist || !originTask.ResultRetrievable() {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	taskResponseFormat := common.GetContextKeyString(c, constant.ContextKeyTaskResponseFormat)
	if taskResponseFormat != "" {
		if native, ok := GetTaskAdaptor(originTask.Platform).(channel.NativeTaskProtocol); ok {
			if !native.SupportsNativeTaskFormat(taskResponseFormat) {
				return nil, service.TaskErrorWrapperLocal(errors.New("task does not support the requested native video protocol"), "not_implemented", http.StatusNotImplemented)
			}
			if taskResponseFormat == constant.TaskResponseFormatMiniMaxVideoV2 && originTask.Properties.OriginModelName != "MiniMax-H3" && originTask.Properties.UpstreamModelName != "MiniMax-H3" {
				return nil, service.TaskErrorWrapperLocal(errors.New("task was not created with the MiniMax video V2 protocol"), "invalid_model", http.StatusBadRequest)
			}
			respBody, err = native.RenderNativeTask(c, taskResponseFormat, originTask)
			if err != nil {
				return nil, service.TaskErrorWrapper(err, "convert_to_native_video_failed", http.StatusInternalServerError)
			}
			return respBody, nil
		}
	}
	if taskResponseFormat == constant.TaskResponseFormatMiniMaxVideoV2 {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
		}
		converter, ok := adaptor.(channel.MiniMaxVideoV2Converter)
		if !ok {
			return nil, service.TaskErrorWrapperLocal(errors.New("task does not support the MiniMax video V2 protocol"), "not_implemented", http.StatusNotImplemented)
		}
		if !converter.IsMiniMaxVideoV2Task(originTask) {
			return nil, service.TaskErrorWrapperLocal(errors.New("task was not created with the MiniMax video V2 protocol"), "invalid_model", http.StatusBadRequest)
		}
		respBody, err = converter.ConvertToMiniMaxVideoV2(originTask)
		if err != nil {
			return nil, service.TaskErrorWrapper(err, "convert_to_minimax_video_v2_failed", http.StatusInternalServerError)
		}
		return respBody, nil
	}

	if taskResponseFormat == constant.TaskResponseFormatDoubaoVideo {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
		}
		converter, ok := adaptor.(channel.NativeVideoConverter)
		if !ok {
			return nil, service.TaskErrorWrapperLocal(errors.New("task does not support the Doubao video protocol"), "not_implemented", http.StatusNotImplemented)
		}
		respBody, err = converter.ConvertToNativeVideo(originTask)
		if err != nil {
			return nil, service.TaskErrorWrapper(err, "convert_to_native_video_failed", http.StatusInternalServerError)
		}
		return respBody, nil
	} else if taskResponseFormat == constant.TaskResponseFormatAliVideo {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
		}
		converter, ok := adaptor.(channel.AliNativeVideoConverter)
		if !ok {
			return nil, service.TaskErrorWrapperLocal(errors.New("task does not support the Ali video protocol"), "not_implemented", http.StatusNotImplemented)
		}
		respBody, err = converter.ConvertToAliNativeVideo(originTask)
		if err != nil {
			return nil, service.TaskErrorWrapper(err, "convert_to_ali_native_video_failed", http.StatusInternalServerError)
		}
		return respBody, nil
	}

	isOpenAIVideoAPI := strings.HasPrefix(c.Request.RequestURI, "/v1/videos/")

	// Gemini/Vertex 支持实时查询：用户 fetch 时直接从上游拉取最新状态
	if realtimeResp := tryRealtimeFetch(originTask, isOpenAIVideoAPI); len(realtimeResp) > 0 {
		respBody = realtimeResp
		return
	}

	// OpenAI Video API 格式: 走各 adaptor 的 ConvertToOpenAIVideo
	if isOpenAIVideoAPI {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
			return
		}
		if converter, ok := adaptor.(channel.OpenAIVideoConverter); ok {
			openAIVideoData, err := converter.ConvertToOpenAIVideo(originTask)
			if err != nil {
				taskResp = service.TaskErrorWrapper(err, "convert_to_openai_video_failed", http.StatusInternalServerError)
				return
			}
			respBody = openAIVideoData
			return
		}
		taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", originTask.Platform), "not_implemented", http.StatusNotImplemented)
		return
	}

	// 通用 TaskDto 格式
	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(c, originTask),
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

// tryRealtimeFetch 尝试从上游实时拉取 Gemini/Vertex 任务状态。
// 仅当渠道类型为 Gemini 或 Vertex 时触发；其他渠道或出错时返回 nil。
// 当非 OpenAI Video API 时，还会构建自定义格式的响应体。
func tryRealtimeFetch(task *model.Task, isOpenAIVideoAPI bool) []byte {
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil
	}
	if channelModel.Type != constant.ChannelTypeVertexAi && channelModel.Type != constant.ChannelTypeGemini {
		return nil
	}

	baseURL := constant.GetChannelBaseURL(channelModel.Type)
	if channelModel.GetBaseURL() != "" {
		baseURL = channelModel.GetBaseURL()
	}
	proxy := channelModel.GetSetting().Proxy
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
	if adaptor == nil {
		return nil
	}

	resp, err := adaptor.FetchTask(baseURL, channelModel.Key, task, proxy)
	if err != nil || resp == nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	ti, err := adaptor.ParseTaskResult(task, resp, body)
	if err != nil || ti == nil {
		return nil
	}

	snap := task.Snapshot()

	// 将上游最新状态更新到 task
	if ti.Status != "" {
		task.Status = model.TaskStatus(ti.Status)
	}
	if ti.Progress != "" {
		task.Progress = ti.Progress
	}
	if strings.HasPrefix(ti.Url, "data:") {
		// data: URI — kept in Data, not ResultURL
	} else if ti.Url != "" {
		task.PrivateData.ResultURL = ti.Url
	} else if task.Status == model.TaskStatusSuccess {
		// No URL from adaptor — construct proxy URL using public task ID
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	}

	if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	// OpenAI Video API 由调用者的 ConvertToOpenAIVideo 分支处理
	if isOpenAIVideoAPI {
		return nil
	}

	// 非 OpenAI Video API: 构建自定义格式响应
	format := detectVideoFormat(body)
	out := map[string]any{
		"error":    nil,
		"format":   format,
		"metadata": nil,
		"status":   mapTaskStatusToSimple(task.Status),
		"task_id":  task.TaskID,
		"url":      task.GetResultURL(),
	}
	respBody, _ := common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: out,
	})
	return respBody
}

// detectVideoFormat 从 Gemini/Vertex 原始响应中探测视频格式
func detectVideoFormat(rawBody []byte) string {
	var raw map[string]any
	if err := common.Unmarshal(rawBody, &raw); err != nil {
		return "mp4"
	}
	respObj, ok := raw["response"].(map[string]any)
	if !ok {
		return "mp4"
	}
	vids, ok := respObj["videos"].([]any)
	if !ok || len(vids) == 0 {
		return "mp4"
	}
	v0, ok := vids[0].(map[string]any)
	if !ok {
		return "mp4"
	}
	mt, ok := v0["mimeType"].(string)
	if !ok || mt == "" || strings.Contains(mt, "mp4") {
		return "mp4"
	}
	return mt
}

// mapTaskStatusToSimple 将内部 TaskStatus 映射为简化状态字符串
func mapTaskStatusToSimple(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func TaskModel2Dto(c *gin.Context, task *model.Task) *dto.TaskDto {
	return &dto.TaskDto{
		ID:         task.ID,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		TaskID:     task.TaskID,
		Platform:   string(task.Platform),
		UserId:     task.UserId,
		Group:      task.Group,
		ChannelId:  task.ChannelId,
		Quota:      task.Quota,
		Action:     constant.NormalizeTaskAction(task.Action),
		Status:     string(task.Status),
		FailReason: service.TaskFailReasonForClient(c, task.FailReason),
		ResultURL:  task.GetResultURL(),
		SubmitTime: task.SubmitTime,
		StartTime:  task.StartTime,
		FinishTime: task.FinishTime,
		Progress:   task.Progress,
		Properties: task.Properties,
		Username:   task.Username,
		Usage:      task.Usage,
		Data:       service.TaskResponseDataForClient(c, task.Data),
	}
}
