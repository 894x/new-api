package hailuo

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

// https://platform.minimaxi.com/docs/api-reference/video-generation-intro
type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *taskdto.TaskError) {
	if taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = req.Model
	}
	if common.GetContextKeyString(c, constant.ContextKeyTaskResponseFormat) == constant.TaskResponseFormatMiniMaxVideoV2 && modelName != H3Model {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("MiniMax video V2 currently supports model %s", H3Model),
			"invalid_model",
			http.StatusBadRequest,
		)
	}
	if modelName == H3Model {
		request, buildErr := buildH3VideoRequest(
			&req,
			H3Model,
			common.GetContextKeyString(c, constant.ContextKeyTaskResponseFormat) == constant.TaskResponseFormatMiniMaxVideoV2,
		)
		if buildErr != nil {
			return service.TaskErrorWrapperLocal(buildErr, "invalid_request", http.StatusBadRequest)
		}
		if h3Scenario(request.Content) == h3ScenarioText {
			info.Action = constant.TaskActionTextGenerate
		}
	}
	return nil
}

func (a *TaskAdaptor) ValidateMappedRequest(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	if !usesH3Protocol(info.OriginModelName, info.UpstreamModelName) {
		return nil
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	request, err := buildH3VideoRequest(
		&req,
		info.UpstreamModelName,
		common.GetContextKeyString(c, constant.ContextKeyTaskResponseFormat) == constant.TaskResponseFormatMiniMaxVideoV2,
	)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if h3Scenario(request.Content) == h3ScenarioText {
		info.Action = constant.TaskActionTextGenerate
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if usesH3Protocol(info.OriginModelName, info.UpstreamModelName) {
		return fmt.Sprintf("%s%s", a.baseURL, H3VideoEndpoint), nil
	}
	return fmt.Sprintf("%s%s", a.baseURL, TextToVideoEndpoint), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("invalid request type in context")
	}

	var body any
	var err error
	if usesH3Protocol(info.OriginModelName, info.UpstreamModelName) {
		body, err = buildH3VideoRequest(
			&req,
			info.UpstreamModelName,
			common.GetContextKeyString(c, constant.ContextKeyTaskResponseFormat) == constant.TaskResponseFormatMiniMaxVideoV2,
		)
	} else {
		body, err = a.convertToRequestPayload(&req, info)
	}
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()
	if usesH3Protocol(info.OriginModelName, info.UpstreamModelName) {
		var hResp H3VideoResponse
		if err := common.Unmarshal(responseBody, &hResp); err != nil {
			taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
			return
		}
		if hResp.Error != nil && strings.TrimSpace(hResp.Error.Message) != "" {
			statusCode := h3APIStatusCode(hResp.Error)
			if statusCode < 400 || statusCode > 599 {
				statusCode = resp.StatusCode
			}
			if statusCode < 400 || statusCode > 599 {
				statusCode = http.StatusBadGateway
			}
			taskErr = service.TaskErrorWrapper(errors.New(hResp.Error.Message), hResp.Error.Type, statusCode)
			return
		}
		if hResp.BaseResp != nil && hResp.BaseResp.StatusCode != StatusSuccess {
			taskErr = service.TaskErrorWrapper(
				fmt.Errorf("hailuo api error: %s", hResp.BaseResp.StatusMsg),
				strconv.Itoa(hResp.BaseResp.StatusCode),
				http.StatusBadRequest,
			)
			return
		}
		if strings.TrimSpace(hResp.TaskID) == "" {
			taskErr = service.TaskErrorWrapper(errors.New("missing task_id"), "invalid_upstream_response", http.StatusBadGateway)
			return
		}

		taskData = a.SanitizeTaskData(responseBody, info.PublicTaskID)
		if common.GetContextKeyString(c, constant.ContextKeyTaskResponseFormat) == constant.TaskResponseFormatMiniMaxVideoV2 {
			c.JSON(http.StatusOK, gin.H{"task_id": info.PublicTaskID})
		} else {
			ov := dto.NewOpenAIVideo()
			ov.ID = info.PublicTaskID
			ov.TaskID = info.PublicTaskID
			ov.CreatedAt = time.Now().Unix()
			ov.Model = info.OriginModelName
			c.JSON(http.StatusOK, ov)
		}
		return hResp.TaskID, taskData, nil
	}

	var hResp VideoResponse
	if err := common.Unmarshal(responseBody, &hResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if hResp.BaseResp.StatusCode != StatusSuccess {
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("hailuo api error: %s", hResp.BaseResp.StatusMsg),
			strconv.Itoa(hResp.BaseResp.StatusCode),
			http.StatusBadRequest,
		)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return hResp.TaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	modelName, _ := body["model"].(string)
	originModelName, _ := body["origin_model"].(string)
	uri := ""
	if usesH3Protocol(originModelName, modelName) {
		uri = fmt.Sprintf("%s%s/%s", baseUrl, H3QueryTaskEndpoint, url.PathEscape(taskID))
	} else {
		uri = fmt.Sprintf("%s%s?task_id=%s", baseUrl, QueryTaskEndpoint, url.QueryEscape(taskID))
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*VideoRequest, error) {
	modelConfig := GetModelConfig(info.UpstreamModelName)
	duration := DefaultDuration
	if req.Duration > 0 {
		duration = req.Duration
	}
	resolution := modelConfig.DefaultResolution
	if req.Size != "" {
		resolution = a.parseResolutionFromSize(req.Size, modelConfig)
	}

	videoRequest := &VideoRequest{
		Model:      info.UpstreamModelName,
		Prompt:     req.Prompt,
		Duration:   &duration,
		Resolution: resolution,
	}
	if err := req.UnmarshalMetadata(&videoRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata to video request failed")
	}

	return videoRequest, nil
}

func (a *TaskAdaptor) parseResolutionFromSize(size string, modelConfig ModelConfig) string {
	switch {
	case strings.Contains(size, "1080"):
		return Resolution1080P
	case strings.Contains(size, "768"):
		return Resolution768P
	case strings.Contains(size, "720"):
		return Resolution720P
	case strings.Contains(size, "512"):
		return Resolution512P
	default:
		return modelConfig.DefaultResolution
	}
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var h3Response H3QueryResponse
	if err := common.Unmarshal(respBody, &h3Response); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	if h3Response.Error != nil && strings.TrimSpace(h3Response.Error.Message) != "" {
		statusCode := h3APIStatusCode(h3Response.Error)
		if statusCode == http.StatusRequestTimeout || statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError {
			return nil, errors.New(h3Response.Error.Message)
		}
		return &relaycommon.TaskInfo{
			Code:     statusCode,
			Status:   string(model.TaskStatusFailure),
			Progress: "100%",
			Reason:   h3Response.Error.Message,
		}, nil
	}
	if h3Response.Task != nil {
		taskResult := &relaycommon.TaskInfo{TaskID: h3Response.Task.ID, Code: 0}
		switch h3Response.Task.Status {
		case H3TaskStatusQueued:
			taskResult.Status = string(model.TaskStatusQueued)
			taskResult.Progress = "30%"
		case H3TaskStatusRunning:
			taskResult.Status = string(model.TaskStatusInProgress)
			taskResult.Progress = "50%"
		case H3TaskStatusSuccess:
			taskResult.Status = string(model.TaskStatusSuccess)
			taskResult.Progress = "100%"
			if h3Response.Task.Content != nil {
				taskResult.Url = strings.TrimSpace(h3Response.Task.Content.URL)
			}
		case H3TaskStatusFailed, H3TaskStatusCanceled:
			taskResult.Status = string(model.TaskStatusFailure)
			taskResult.Progress = "100%"
			if h3Response.Task.Error != nil {
				taskResult.Reason = strings.TrimSpace(h3Response.Task.Error.Message)
			}
			if taskResult.Reason == "" {
				taskResult.Reason = "task " + h3Response.Task.Status
			}
		default:
			taskResult.Status = string(model.TaskStatusUnknown)
			taskResult.Reason = "unrecognized status: " + h3Response.Task.Status
		}
		return taskResult, nil
	}

	resTask := QueryTaskResponse{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{}

	if resTask.BaseResp.StatusCode == StatusSuccess {
		taskResult.Code = 0
	} else {
		taskResult.Code = resTask.BaseResp.StatusCode
		taskResult.Reason = resTask.BaseResp.StatusMsg
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
	}

	switch resTask.Status {
	case TaskStatusPreparing, TaskStatusQueueing, TaskStatusProcessing:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
		if resTask.Status == TaskStatusProcessing {
			taskResult.Progress = "50%"
		}
	case TaskStatusSuccess:
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = a.buildVideoURL(resTask.TaskID, resTask.FileID)
	case TaskStatusFailed:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		if taskResult.Reason == "" {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) SanitizeTaskData(body []byte, publicTaskID string) []byte {
	if len(body) == 0 || publicTaskID == "" {
		return body
	}
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return body
	}
	changed := false
	if _, ok := payload["task_id"]; ok {
		payload["task_id"] = publicTaskID
		changed = true
	}
	if task, ok := payload["task"].(map[string]any); ok {
		task["id"] = publicTaskID
		changed = true
	}
	if !changed {
		return body
	}
	sanitized, err := common.Marshal(payload)
	if err != nil {
		return body
	}
	return sanitized
}

func (a *TaskAdaptor) ConvertToMiniMaxVideoV2(originTask *model.Task) ([]byte, error) {
	if originTask == nil {
		return nil, errors.New("task is nil")
	}
	if !a.IsMiniMaxVideoV2Task(originTask) {
		return nil, errors.New("task was not created with the MiniMax video V2 protocol")
	}

	var storedPayload map[string]any
	if err := common.Unmarshal(originTask.Data, &storedPayload); err == nil {
		if task, ok := storedPayload["task"].(map[string]any); ok {
			task["id"] = originTask.TaskID
			if originTask.Properties.OriginModelName != "" {
				task["model"] = originTask.Properties.OriginModelName
			}
			return common.Marshal(storedPayload)
		}
	}

	createdAt := originTask.CreatedAt
	if createdAt == 0 {
		createdAt = originTask.SubmitTime
	}
	updatedAt := originTask.UpdatedAt
	if updatedAt == 0 {
		updatedAt = createdAt
	}
	task := map[string]any{
		"id":         originTask.TaskID,
		"model":      originTask.Properties.OriginModelName,
		"status":     miniMaxV2TaskStatus(originTask.Status),
		"created_at": createdAt,
		"updated_at": updatedAt,
	}
	if resultURL := originTask.GetResultURL(); resultURL != "" && originTask.Status == model.TaskStatusSuccess {
		task["content"] = map[string]any{"url": resultURL}
	}
	if originTask.Status == model.TaskStatusFailure {
		task["error"] = map[string]any{
			"code":    "task_failed",
			"message": originTask.FailReason,
		}
	}
	return common.Marshal(map[string]any{"task": task})
}

func (a *TaskAdaptor) IsMiniMaxVideoV2Task(originTask *model.Task) bool {
	if originTask == nil {
		return false
	}
	return usesH3Protocol(originTask.Properties.OriginModelName, originTask.Properties.UpstreamModelName)
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var hailuoResp QueryTaskResponse
	if err := common.Unmarshal(originTask.Data, &hailuoResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal hailuo task data failed")
	}

	openAIVideo := originTask.ToOpenAIVideo()
	if hailuoResp.BaseResp.StatusCode != StatusSuccess {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: hailuoResp.BaseResp.StatusMsg,
			Code:    strconv.Itoa(hailuoResp.BaseResp.StatusCode),
		}
	}

	jsonData, err := common.Marshal(openAIVideo)
	if err != nil {
		return nil, errors.Wrap(err, "marshal openai video failed")
	}

	return jsonData, nil
}

func miniMaxV2TaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusInProgress:
		return H3TaskStatusRunning
	case model.TaskStatusSuccess:
		return H3TaskStatusSuccess
	case model.TaskStatusFailure:
		return H3TaskStatusFailed
	default:
		return H3TaskStatusQueued
	}
}

func usesH3Protocol(originModelName string, upstreamModelName string) bool {
	return originModelName == H3Model || upstreamModelName == H3Model
}

func (a *TaskAdaptor) buildVideoURL(_, fileID string) string {
	if a.apiKey == "" || a.baseURL == "" {
		return ""
	}

	url := fmt.Sprintf("%s/v1/files/retrieve?file_id=%s", a.baseURL, fileID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var retrieveResp RetrieveFileResponse
	if err := common.Unmarshal(responseBody, &retrieveResp); err != nil {
		return ""
	}

	if retrieveResp.BaseResp.StatusCode != StatusSuccess {
		return ""
	}

	return retrieveResp.File.DownloadURL
}

func buildH3VideoRequest(req *relaycommon.TaskSubmitReq, modelName string, officialRequest bool) (*H3VideoRequest, error) {
	content, err := h3Content(req)
	if err != nil {
		return nil, err
	}
	duration, err := h3Duration(req, officialRequest)
	if err != nil {
		return nil, err
	}
	resolution, err := h3Resolution(req, officialRequest)
	if err != nil {
		return nil, err
	}
	ratio, err := h3Ratio(req, content, officialRequest)
	if err != nil {
		return nil, err
	}

	request := &H3VideoRequest{
		Model:      modelName,
		Content:    content,
		Resolution: resolution,
		Duration:   duration,
		Ratio:      ratio,
	}
	if value, ok := req.Metadata["callback_url"]; ok && value != nil {
		callbackURL, ok := value.(string)
		if !ok {
			return nil, errors.New("callback_url must be a string")
		}
		request.CallbackURL = &callbackURL
	}
	if value, ok := req.Metadata["aigc_watermark"]; ok && value != nil {
		watermark, ok := value.(bool)
		if !ok {
			return nil, errors.New("aigc_watermark must be a boolean")
		}
		request.AIGCWatermark = &watermark
	}
	return request, nil
}

func h3Duration(req *relaycommon.TaskSubmitReq, officialRequest bool) (int, error) {
	var raw any
	provided := false
	if req.Metadata != nil {
		raw, provided = req.Metadata["duration"]
	}
	if !provided && req.Duration != 0 {
		raw = req.Duration
		provided = true
	}
	if !provided || raw == nil || raw == "" {
		if officialRequest {
			return 0, fmt.Errorf("%s duration is required", H3Model)
		}
		return H3DefaultDuration, nil
	}
	if officialRequest {
		duration, ok := h3StrictInteger(raw)
		if !ok || duration < H3MinDuration || duration > H3MaxDuration {
			return 0, fmt.Errorf("%s duration must be an integer between %d and %d seconds", H3Model, H3MinDuration, H3MaxDuration)
		}
		return duration, nil
	}
	duration, err := strconv.Atoi(fmt.Sprint(raw))
	if err != nil || duration < H3MinDuration || duration > H3MaxDuration {
		return 0, fmt.Errorf("%s duration must be an integer between %d and %d seconds", H3Model, H3MinDuration, H3MaxDuration)
	}
	return duration, nil
}

func h3Resolution(req *relaycommon.TaskSubmitReq, officialRequest bool) (string, error) {
	var raw any
	if req.Metadata != nil {
		raw = req.Metadata["resolution"]
		if raw == nil && !officialRequest {
			raw = req.Metadata["size"]
		}
	}
	if (raw == nil || raw == "") && !officialRequest {
		raw = req.Size
	}
	if raw == nil || raw == "" {
		if officialRequest {
			return "", fmt.Errorf("%s resolution is required", H3Model)
		}
		return Resolution768P, nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s resolution must be 768P or 2K", H3Model)
	}
	if officialRequest {
		if value == Resolution768P || value == Resolution2K {
			return value, nil
		}
		return "", fmt.Errorf("%s resolution must be 768P or 2K", H3Model)
	}
	value = strings.ToUpper(strings.TrimSpace(value))
	switch {
	case strings.Contains(value, Resolution2K):
		return Resolution2K, nil
	case strings.Contains(value, Resolution768P):
		return Resolution768P, nil
	default:
		return "", fmt.Errorf("%s resolution must be 768P or 2K", H3Model)
	}
}

func h3StrictInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		if typed < math.MinInt || typed > math.MaxInt {
			return 0, false
		}
		return int(typed), true
	case uint:
		if uint64(typed) > uint64(math.MaxInt) {
			return 0, false
		}
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		if typed > uint64(math.MaxInt) {
			return 0, false
		}
		return int(typed), true
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed < math.MinInt || typed > math.MaxInt {
			return 0, false
		}
		return int(typed), true
	case float32:
		value := float64(typed)
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < math.MinInt || value > math.MaxInt {
			return 0, false
		}
		return int(value), true
	default:
		return 0, false
	}
}

func h3Content(req *relaycommon.TaskSubmitReq) ([]any, error) {
	metadata := req.Metadata
	prompt := strings.TrimSpace(req.Prompt)
	if metadata != nil {
		if rawContent, exists := metadata["content"]; exists {
			content, ok := rawContent.([]any)
			if !ok {
				return nil, errors.New("metadata.content must be an array")
			}
			items := append([]any(nil), content...)
			for _, item := range items {
				contentItem, ok := item.(map[string]any)
				if ok && contentItem["type"] == "text" && strings.TrimSpace(common.Interface2String(contentItem["text"])) != "" {
					return validateH3Content(items)
				}
			}
			if prompt == "" {
				return nil, fmt.Errorf("%s metadata.content requires a text item or a prompt", H3Model)
			}
			items = append([]any{map[string]any{"type": "text", "text": prompt}}, items...)
			return validateH3Content(items)
		}
	}

	content := make([]any, 0, 1+len(req.Images))
	if prompt != "" {
		content = append(content, map[string]any{"type": "text", "text": prompt})
	}
	frames, err := h3FrameImages(req)
	if err != nil {
		return nil, err
	}
	content = append(content, frames...)
	if metadata != nil {
		videos := h3MediaValues(metadata["reference_video"])
		if len(videos) > H3MaxReferenceVideos {
			return nil, fmt.Errorf("%s accepts at most %d reference videos", H3Model, H3MaxReferenceVideos)
		}
		for _, video := range videos {
			content = append(content, h3MediaItem("video_url", video, "reference_video"))
		}
		audios := h3MediaValues(metadata["reference_audio"])
		if len(audios) > H3MaxReferenceAudios {
			return nil, fmt.Errorf("%s accepts at most %d reference audios", H3Model, H3MaxReferenceAudios)
		}
		for _, audio := range audios {
			content = append(content, h3MediaItem("audio_url", audio, "reference_audio"))
		}
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("%s requires a prompt or a media input", H3Model)
	}
	return validateH3Content(content)
}

func h3FrameImages(req *relaycommon.TaskSubmitReq) ([]any, error) {
	metadata := req.Metadata
	frames := make([]any, 0, H3MaxFrameImages)
	if metadata != nil {
		if firstFrame, ok := metadata["first_frame_image"]; ok && h3MediaValuePresent(firstFrame) {
			frames = append(frames, h3MediaItem("image_url", firstFrame, "first_frame"))
		}
		if lastFrame, ok := metadata["last_frame_image"]; ok && h3MediaValuePresent(lastFrame) {
			frames = append(frames, h3MediaItem("image_url", lastFrame, "last_frame"))
		}
	}
	if len(frames) > 0 {
		return frames, nil
	}
	if len(req.Images) > H3MaxFrameImages {
		return nil, fmt.Errorf("%s accepts at most %d frame images", H3Model, H3MaxFrameImages)
	}
	for index, image := range req.Images {
		if !h3MediaValuePresent(image) {
			continue
		}
		role := "first_frame"
		if index == 1 {
			role = "last_frame"
		}
		frames = append(frames, h3MediaItem("image_url", image, role))
	}
	return frames, nil
}

func h3MediaValues(raw any) []any {
	var values []any
	switch value := raw.(type) {
	case []any:
		values = value
	case []string:
		values = make([]any, 0, len(value))
		for _, item := range value {
			values = append(values, item)
		}
	case nil:
		return nil
	default:
		values = []any{value}
	}
	filtered := make([]any, 0, len(values))
	for _, value := range values {
		if h3MediaValuePresent(value) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func h3MediaValuePresent(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	_, ok := value.(map[string]any)
	return ok
}

func h3MediaItem(mediaType string, mediaURL any, role string) map[string]any {
	return map[string]any{
		"type": mediaType,
		"role": role,
		mediaType: map[string]any{
			"url": mediaURL,
		},
	}
}

func validateH3Content(items []any) ([]any, error) {
	textCount := 0
	hasFrame := false
	hasReference := false
	firstFrames := 0
	lastFrames := 0
	inputImages := 0
	referenceImages := 0
	referenceVideos := 0
	referenceAudios := 0

	for index, item := range items {
		contentItem, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s content item %d must be an object", H3Model, index)
		}
		itemType, ok := contentItem["type"].(string)
		if !ok || itemType == "" {
			return nil, fmt.Errorf("%s content item %d requires a valid type", H3Model, index)
		}
		role := ""
		if rawRole, exists := contentItem["role"]; exists && rawRole != nil {
			role, ok = rawRole.(string)
			if !ok {
				return nil, fmt.Errorf("%s content item %d role must be a string", H3Model, index)
			}
		}
		switch itemType {
		case "text":
			if role != "" {
				return nil, fmt.Errorf("%s text content cannot have a role", H3Model)
			}
			text, ok := contentItem["text"].(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, fmt.Errorf("%s requires a non-empty text item", H3Model)
			}
			if utf8.RuneCountInString(text) > H3MaxPromptCharacters {
				return nil, fmt.Errorf("%s text must not exceed %d characters", H3Model, H3MaxPromptCharacters)
			}
			textCount++
		case "image_url":
			if err := validateH3MediaURL(contentItem, itemType, index); err != nil {
				return nil, err
			}
			inputImages++
			switch role {
			case "", "first_frame":
				firstFrames++
				hasFrame = true
			case "last_frame":
				lastFrames++
				hasFrame = true
			case "reference_image":
				referenceImages++
				hasReference = true
			default:
				return nil, fmt.Errorf("%s image_url role must be first_frame, last_frame, or reference_image", H3Model)
			}
		case "video_url":
			if role != "reference_video" {
				return nil, fmt.Errorf("%s video_url role must be reference_video", H3Model)
			}
			if err := validateH3MediaURL(contentItem, itemType, index); err != nil {
				return nil, err
			}
			referenceVideos++
			hasReference = true
		case "audio_url":
			if role != "reference_audio" {
				return nil, fmt.Errorf("%s audio_url role must be reference_audio", H3Model)
			}
			if err := validateH3MediaURL(contentItem, itemType, index); err != nil {
				return nil, err
			}
			referenceAudios++
			hasReference = true
		default:
			return nil, fmt.Errorf("%s content item %d has unsupported type %q", H3Model, index, itemType)
		}
	}

	switch {
	case textCount == 0:
		return nil, fmt.Errorf("%s requires a non-empty text item", H3Model)
	case textCount > 1:
		return nil, fmt.Errorf("%s requires exactly one text item", H3Model)
	case firstFrames > 1:
		return nil, fmt.Errorf("%s accepts at most one first_frame image", H3Model)
	case lastFrames > 1:
		return nil, fmt.Errorf("%s accepts at most one last_frame image", H3Model)
	case referenceImages > H3MaxReferenceImages:
		return nil, fmt.Errorf("%s accepts at most %d reference images", H3Model, H3MaxReferenceImages)
	case inputImages > H3MaxReferenceImages:
		return nil, fmt.Errorf("%s accepts at most %d input images", H3Model, H3MaxReferenceImages)
	case referenceVideos > H3MaxReferenceVideos:
		return nil, fmt.Errorf("%s accepts at most %d reference videos", H3Model, H3MaxReferenceVideos)
	case referenceAudios > H3MaxReferenceAudios:
		return nil, fmt.Errorf("%s accepts at most %d reference audios", H3Model, H3MaxReferenceAudios)
	case hasFrame && hasReference:
		return nil, fmt.Errorf("%s cannot mix frame images with reference media", H3Model)
	}
	return items, nil
}

func validateH3MediaURL(contentItem map[string]any, mediaType string, index int) error {
	media, ok := contentItem[mediaType].(map[string]any)
	if !ok {
		return fmt.Errorf("%s content item %d requires %s.url", H3Model, index, mediaType)
	}
	mediaURL, ok := media["url"].(string)
	if !ok || strings.TrimSpace(mediaURL) == "" {
		return fmt.Errorf("%s content item %d requires a non-empty %s.url", H3Model, index, mediaType)
	}
	return nil
}

type h3GenerationScenario int

const (
	h3ScenarioText h3GenerationScenario = iota
	h3ScenarioFrame
	h3ScenarioReference
)

func h3Scenario(content []any) h3GenerationScenario {
	for _, item := range content {
		contentItem, _ := item.(map[string]any)
		itemType, _ := contentItem["type"].(string)
		role, _ := contentItem["role"].(string)
		if itemType == "video_url" || itemType == "audio_url" || role == "reference_image" {
			return h3ScenarioReference
		}
		if itemType == "image_url" {
			return h3ScenarioFrame
		}
	}
	return h3ScenarioText
}

func h3Ratio(req *relaycommon.TaskSubmitReq, content []any, officialRequest bool) (string, error) {
	ratio := ""
	provided := false
	if req.Metadata != nil {
		if raw, ok := req.Metadata["ratio"]; ok && raw != nil {
			provided = true
			var isString bool
			ratio, isString = raw.(string)
			if !isString {
				return "", fmt.Errorf("%s ratio must be one of %s", H3Model, strings.Join(H3Ratios, ", "))
			}
			if !officialRequest {
				ratio = strings.TrimSpace(ratio)
			}
		}
	}
	if provided && !contains(H3Ratios, ratio) {
		return "", fmt.Errorf("%s ratio must be one of %s", H3Model, strings.Join(H3Ratios, ", "))
	}
	switch h3Scenario(content) {
	case h3ScenarioFrame:
		return "adaptive", nil
	case h3ScenarioReference:
		if !provided || ratio == "" {
			return "adaptive", nil
		}
		return ratio, nil
	default:
		if !provided || ratio == "" {
			if officialRequest {
				return "", fmt.Errorf("%s ratio is required for text-to-video", H3Model)
			}
			return "16:9", nil
		}
		if ratio == "adaptive" {
			return "", fmt.Errorf("%s ratio adaptive requires an image, video, or audio input", H3Model)
		}
		return ratio, nil
	}
}

func h3APIStatusCode(apiError *H3APIError) int {
	if apiError == nil {
		return 0
	}
	for _, value := range []any{apiError.HTTPCode, apiError.Code} {
		statusCode, err := strconv.Atoi(fmt.Sprint(value))
		if err == nil {
			return statusCode
		}
	}
	return 0
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsInt(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
