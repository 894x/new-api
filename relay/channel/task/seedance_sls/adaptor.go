package seedance_sls

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const nativeRequestContextKey = "seedance_sls_native_request"

var modelList = []string{
	"doubao-seedance-2-0",
	"doubao-seedance-2-0-fast",
	"doubao-seedance-2-0-mini",
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

type taskResponse struct {
	TaskID      string          `json:"task_id"`
	Status      string          `json:"status"`
	FailReason  string          `json:"fail_reason"`
	ResultURL   string          `json:"result_url"`
	Progress    string          `json:"progress"`
	TotalTokens int             `json:"total_tokens"`
	Data        json.RawMessage `json:"data"`
}

type wrappedTaskResponse struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Data    taskResponse `json:"data"`
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	if !strings.HasPrefix(c.GetHeader("Content-Type"), gin.MIMEJSON) {
		return localTaskError(fmt.Errorf("Seedance SLS requires application/json requests"), "invalid_request")
	}

	var payload map[string]any
	if err := common.UnmarshalBodyReusable(c, &payload); err != nil {
		return localTaskError(err, "invalid_request")
	}
	contentValue, nativeRequest := payload["content"]
	if !nativeRequest {
		if info.TaskRelayInfo == nil {
			info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
		}
		if taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
			return taskErr
		}
		request, err := relaycommon.GetTaskRequest(c)
		if err != nil {
			return localTaskError(err, "invalid_request")
		}
		if request.Duration == 0 && request.Seconds == "" {
			if metadataDuration, exists := request.Metadata["duration"]; exists {
				duration, err := requestDuration(metadataDuration)
				if err != nil || duration < 0 || duration > relaycommon.MaxTaskDurationSeconds {
					if err == nil {
						err = fmt.Errorf("duration must be between 0 and %d", relaycommon.MaxTaskDurationSeconds)
					}
					return localTaskError(err, "invalid_seconds")
				}
				request.Duration = duration
				c.Set("task_request", request)
			}
		}
		return nil
	}

	content, ok := contentValue.([]any)
	if !ok {
		return localTaskError(fmt.Errorf("content must be an array"), "invalid_request")
	}
	textParts := make([]string, 0, len(content))
	for _, itemValue := range content {
		item, ok := itemValue.(map[string]any)
		if !ok || item["type"] != "text" {
			continue
		}
		if text, ok := item["text"].(string); ok && strings.TrimSpace(text) != "" {
			textParts = append(textParts, text)
		}
	}
	prompt := strings.Join(textParts, "\n")
	if strings.TrimSpace(prompt) == "" {
		return localTaskError(fmt.Errorf("content must contain a non-empty text item"), "invalid_request")
	}

	duration, err := requestDuration(payload["duration"])
	if err != nil {
		return localTaskError(err, "invalid_seconds")
	}
	if duration < 0 || duration > relaycommon.MaxTaskDurationSeconds {
		return localTaskError(fmt.Errorf("duration must be between 0 and %d", relaycommon.MaxTaskDurationSeconds), "invalid_seconds")
	}

	modelName, _ := payload["model"].(string)
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	info.Action = constant.TaskActionGenerate
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt:   prompt,
		Model:    modelName,
		Duration: duration,
	})
	c.Set(nativeRequestContextKey, payload)
	return nil
}

func localTaskError(err error, code string) *taskdto.TaskError {
	return &taskdto.TaskError{
		Code:       code,
		Message:    err.Error(),
		StatusCode: http.StatusBadRequest,
		LocalError: true,
		Error:      err,
	}
}

func requestDuration(value any) (int, error) {
	switch duration := value.(type) {
	case nil:
		return 0, nil
	case float64:
		if duration < 0 || duration > relaycommon.MaxTaskDurationSeconds {
			return 0, fmt.Errorf("duration must be between 0 and %d", relaycommon.MaxTaskDurationSeconds)
		}
		if duration != float64(int(duration)) {
			return 0, fmt.Errorf("duration must be an integer")
		}
		return int(duration), nil
	case string:
		parsed, err := strconv.Atoi(duration)
		if err != nil {
			return 0, fmt.Errorf("duration must be an integer")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("duration must be an integer")
	}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return strings.TrimRight(a.baseURL, "/") + "/v1/video/generations", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	request, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	payload := make(map[string]any)
	if nativeValue, ok := c.Get(nativeRequestContextKey); ok {
		nativePayload, ok := nativeValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Seedance SLS request payload")
		}
		for key, value := range nativePayload {
			payload[key] = value
		}
	} else {
		for key, value := range request.Metadata {
			if key != "model" && key != "content" && key != "duration" && key != "seconds" {
				payload[key] = value
			}
		}
		content := make([]any, 0, len(request.Images)+1)
		for _, imageURL := range request.Images {
			content = append(content, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": imageURL},
			})
		}
		content = append(content, map[string]any{"type": "text", "text": request.Prompt})
		payload["content"] = content
		if request.Duration > 0 {
			payload["duration"] = request.Duration
		} else if seconds, parseErr := strconv.Atoi(request.Seconds); parseErr == nil && seconds > 0 {
			payload["duration"] = seconds
		}
	}

	modelName := request.Model
	if info.ChannelMeta != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	} else if info.ChannelMeta != nil {
		info.UpstreamModelName = modelName
	}
	payload["model"] = modelName
	payload, err = service.RewriteAssetReferences(info.UserId, info.ChannelId, payload)
	if err != nil {
		return nil, errors.Wrap(err, "rewrite asset references failed")
	}

	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var response map[string]any
	if err := common.Unmarshal(responseBody, &response); err != nil {
		return "", responseBody, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	taskID, _ := response["task_id"].(string)
	if taskID == "" {
		taskID, _ = response["id"].(string)
	}
	if taskID == "" {
		return "", responseBody, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	publicTaskID := info.PublicTaskID
	if common.GetContextKeyString(c, constant.ContextKeyTaskResponseFormat) == constant.TaskResponseFormatDoubaoVideo {
		c.JSON(http.StatusOK, map[string]any{"id": publicTaskID})
	} else {
		video := relaykitdto.NewOpenAIVideo()
		video.ID = publicTaskID
		video.TaskID = publicTaskID
		video.CreatedAt = time.Now().Unix()
		video.Model = info.OriginModelName
		c.JSON(http.StatusOK, video)
	}
	return taskID, a.SanitizeTaskData(responseBody, publicTaskID), nil
}

func (a *TaskAdaptor) SanitizeTaskData(responseBody []byte, publicTaskID string) []byte {
	var response map[string]any
	if err := common.Unmarshal(responseBody, &response); err != nil {
		return responseBody
	}
	if _, exists := response["task_id"]; exists {
		response["task_id"] = publicTaskID
	}
	if _, exists := response["id"]; exists {
		response["id"] = publicTaskID
	}
	replaceUpstreamTaskIDs(response, publicTaskID)
	sanitized, err := common.Marshal(response)
	if err != nil {
		return responseBody
	}
	return sanitized
}

func replaceUpstreamTaskIDs(value any, publicTaskID string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "task_id" || key == "upstream_task_id" {
				typed[key] = publicTaskID
				continue
			}
			replaceUpstreamTaskIDs(child, publicTaskID)
		}
	case []any:
		for _, child := range typed {
			replaceUpstreamTaskIDs(child, publicTaskID)
		}
	}
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	uri := strings.TrimRight(baseURL, "/") + "/v1/video/generations/" + url.PathEscape(taskID)
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

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	return a.parseTaskResult(respBody, 0)
}

func (a *TaskAdaptor) parseTaskResult(respBody []byte, depth int) (*relaycommon.TaskInfo, error) {
	var wrapped wrappedTaskResponse
	if err := common.Unmarshal(respBody, &wrapped); err != nil {
		return nil, errors.Wrap(err, "unmarshal Seedance SLS task result failed")
	}
	if wrapped.Code != "" && wrapped.Code != taskdto.TaskSuccessCode {
		return nil, fmt.Errorf("Seedance SLS task query failed: %s", wrapped.Message)
	}
	result := wrapped.Data
	if wrapped.Code == "" {
		if err := common.Unmarshal(respBody, &result); err != nil {
			return nil, errors.Wrap(err, "unmarshal Seedance SLS task data failed")
		}
	}

	taskInfo := &relaycommon.TaskInfo{
		TaskID:      result.TaskID,
		Status:      result.Status,
		Reason:      result.FailReason,
		Url:         result.ResultURL,
		Progress:    result.Progress,
		TotalTokens: result.TotalTokens,
	}
	switch strings.ToUpper(result.Status) {
	case "SUBMITTED", "PENDING":
		taskInfo.Status = string(model.TaskStatusSubmitted)
		if taskInfo.Progress == "" {
			taskInfo.Progress = taskcommon.ProgressSubmitted
		}
	case "QUEUED":
		taskInfo.Status = string(model.TaskStatusQueued)
		if taskInfo.Progress == "" {
			taskInfo.Progress = taskcommon.ProgressQueued
		}
	case "IN_PROGRESS", "PROCESSING", "RUNNING":
		taskInfo.Status = string(model.TaskStatusInProgress)
		if taskInfo.Progress == "" {
			taskInfo.Progress = taskcommon.ProgressInProgress
		}
	case "SUCCESS", "SUCCEEDED", "COMPLETED":
		taskInfo.Status = string(model.TaskStatusSuccess)
		if taskInfo.Progress == "" {
			taskInfo.Progress = taskcommon.ProgressComplete
		}
	case "FAILURE", "FAILED":
		taskInfo.Status = string(model.TaskStatusFailure)
		if taskInfo.Progress == "" {
			taskInfo.Progress = taskcommon.ProgressComplete
		}
	}

	if len(result.Data) > 0 && depth < 4 && string(result.Data) != "null" {
		nested, err := a.parseTaskResult(result.Data, depth+1)
		if err != nil {
			return nil, err
		}
		if taskInfo.TaskID == "" {
			taskInfo.TaskID = nested.TaskID
		}
		if taskInfo.Status == "" {
			taskInfo.Status = nested.Status
		}
		if taskInfo.Reason == "" {
			taskInfo.Reason = nested.Reason
		}
		if taskInfo.Url == "" {
			taskInfo.Url = nested.Url
		}
		if taskInfo.RemoteUrl == "" {
			taskInfo.RemoteUrl = nested.RemoteUrl
		}
		if taskInfo.Progress == "" {
			taskInfo.Progress = nested.Progress
		}
		if taskInfo.CompletionTokens == 0 {
			taskInfo.CompletionTokens = nested.CompletionTokens
		}
		if taskInfo.TotalTokens == 0 {
			taskInfo.TotalTokens = nested.TotalTokens
		}
	}
	return taskInfo, nil
}

func (a *TaskAdaptor) ParseWrappedTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	return a.ParseTaskResult(respBody)
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	video := originTask.ToOpenAIVideo()
	video.TaskID = originTask.TaskID
	if originTask.Status == model.TaskStatusFailure {
		video.Error = &relaykitdto.OpenAIVideoError{Message: originTask.FailReason}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string {
	return modelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return "seedance-sls"
}
