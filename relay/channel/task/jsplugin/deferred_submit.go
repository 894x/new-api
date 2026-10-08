package jsplugin

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayparam"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func (a *TaskAdaptor) PrepareTaskSubmission(c *gin.Context, info *relaycommon.RelayInfo) (*model.TaskPendingSubmission, error) {
	if a.plugin.Meta.Key != "seedance-sls" {
		return nil, nil
	}
	descriptor, err := a.buildSubmit(c, info)
	if err != nil {
		return nil, err
	}
	payload, err := inlineJSONFilePlaceholders(c, descriptor.Body)
	if err != nil {
		return nil, err
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	if int64(len(body)) > int64(maxMB)<<20 {
		return nil, common.ErrRequestBodyTooLarge
	}
	pending := &model.TaskPendingSubmission{Body: body, BaseURL: info.ChannelBaseUrl, Proxy: info.ChannelSetting.Proxy}
	convertMedia := info.ChannelOtherSettings.SeedanceBase64VideoToURL == nil || *info.ChannelOtherSettings.SeedanceBase64VideoToURL
	pending.ConvertBase64Media = &convertMedia
	if info.ChannelOtherSettings.ParameterCapabilities != nil {
		pending.MediaPolicy, err = common.Marshal(info.ChannelOtherSettings.ParameterCapabilities)
		if err != nil {
			return nil, err
		}
	}
	return pending, nil
}

// SubmitPendingTask consumes only persisted data and the runner's bounded
// context. The caller's HTTP request and Gin context are never retained.
func (a *TaskAdaptor) SubmitPendingTask(ctx context.Context, task *model.Task) (*service.PendingTaskSubmitResult, error) {
	if a.plugin.Meta.Key != "seedance-sls" {
		return nil, fmt.Errorf("pending submission requires the Seedance SLS plugin")
	}
	pending := task.PrivateData.PendingSubmission
	leaseID := pending.LeaseID
	var payload map[string]any
	if err := common.Unmarshal(pending.Body, &payload); err != nil {
		return nil, err
	}
	info := &relaycommon.RelayInfo{UserId: task.UserId, OriginModelName: task.Properties.OriginModelName,
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: task.TaskID, Action: task.Action},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: task.ChannelId, ChannelType: a.info.ChannelType,
			UpstreamModelName: task.Properties.UpstreamModelName,
			ChannelBaseUrl:    pending.BaseURL, ApiKey: task.PrivateData.Key,
			ChannelSetting: a.info.ChannelSetting, ChannelOtherSettings: a.info.ChannelOtherSettings}}
	info.ChannelSetting.Proxy = pending.Proxy
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, pending.BaseURL, nil)
	if err != nil {
		return nil, err
	}
	request.Body = http.NoBody
	c := &gin.Context{Request: request}
	if task.PrivateData.Execution != nil {
		c.Set(common.RequestIdKey, task.PrivateData.Execution.RequestID)
	}
	storage, err := system_setting.LoadAssetStorageConfig()
	if err != nil {
		return nil, err
	}
	if storage.Enabled {
		references := service.ManagedAssetReferences{}
		payload, err = service.StoreVideoAssetReferences(ctx, task.UserId, payload, references, true)
		if err != nil {
			return nil, err
		}
		if err := service.RejectAssetReferences(payload); err != nil {
			return nil, err
		}
		task.PrivateData.AssetReferences = &model.TaskAssetReferences{}
		for _, asset := range references {
			task.PrivateData.AssetReferences.Items = append(task.PrivateData.AssetReferences.Items,
				model.TaskAssetReference{AssetID: asset.Id, StoredObjectID: asset.StoredObjectId})
		}
		sort.Slice(task.PrivateData.AssetReferences.Items, func(i, j int) bool {
			return task.PrivateData.AssetReferences.Items[i].AssetID < task.PrivateData.AssetReferences.Items[j].AssetID
		})
	}
	if len(pending.MediaPolicy) > 0 {
		var policy kitdto.ParameterCapabilityConfig
		if err := common.Unmarshal(pending.MediaPolicy, &policy); err != nil {
			return nil, err
		}
		data, err := common.Marshal(payload)
		if err != nil {
			return nil, err
		}
		transform := service.NewParameterMediaTransformer(c, info)
		data, _, err = relayparam.ApplyMediaDelivery(data, &policy, info.UpstreamModelName, info.MediaProcessor)
		if err != nil {
			return nil, err
		}
		data, _, err = relayparam.ApplyMediaTransforms(data, &policy, info.UpstreamModelName, transform)
		if err != nil {
			return nil, err
		}
		if err := common.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
	}
	convertMedia := pending.ConvertBase64Media == nil || *pending.ConvertBase64Media
	payload, err = service.ConvertSeedanceBase64Media(ctx, payload, convertMedia)
	if err != nil {
		return nil, err
	}
	ctx, err = service.ValidateSeedanceMedia(ctx, task.UserId, task.Properties.UpstreamModelName, payload)
	if err != nil {
		return nil, err
	}
	payload, err = service.PrepareAssetReferences(ctx, task.UserId, task.ChannelId, payload)
	if err != nil {
		return nil, err
	}
	c.Request = c.Request.WithContext(ctx)
	c.Set("task_request", map[string]any{"payload": payload})
	a.submit = nil
	a.requestPrepared = false
	a.modelMappingResolved = true
	descriptor, err := a.buildSubmit(c, info)
	if err != nil {
		return nil, err
	}
	descriptor.Body = payload
	a.requestPrepared = true
	body, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	// This barrier precedes the billable POST. A crash or ambiguous transport
	// outcome after it must never trigger an automatic duplicate submission.
	pending.Stage = model.TaskSubmissionUpstream
	task.Status = model.TaskStatusSubmitted
	won, err := model.UpdateClaimedTaskSubmission(ctx, task, leaseID)
	if err != nil {
		return nil, err
	}
	if !won {
		return nil, fmt.Errorf("pending task submission lost its lease")
	}
	resp, err := a.DoRequest(c, info, bytes.NewReader(body))
	if err != nil {
		return nil, &service.PendingTaskSubmitError{Err: err, Unknown: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		failure := a.ParseSubmitError(c, resp, info)
		return nil, fmt.Errorf("%s", failure.Message)
	}
	parsed, taskErr := a.ParseResponse(c, resp, info)
	if taskErr != nil {
		return nil, &service.PendingTaskSubmitError{Err: fmt.Errorf("%s", taskErr.Message), Unknown: true}
	}
	if parsed.Immediate != nil {
		return nil, &service.PendingTaskSubmitError{Err: fmt.Errorf("SLS returned an unexpected immediate result"), Unknown: true}
	}
	return &service.PendingTaskSubmitResult{UpstreamTaskID: parsed.UpstreamTaskID, TaskData: parsed.TaskData, PluginState: parsed.PluginState}, nil
}
