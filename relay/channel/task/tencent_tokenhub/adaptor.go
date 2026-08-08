package tencent_tokenhub

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	ChannelName            = "tencent_tokenhub"
	ModelHYVideo15         = "hy-video-1.5"
	ModelYTVideo20         = "yt-video-2.0"
	ModelYTVideoFX         = "yt-video-fx"
	ModelYTVideoHumanActor = "yt-video-humanactor"
	ModelKLVideoV3         = "kl-video-v3"
	ModelKLVideoV26        = "kl-video-v2-6"
	ModelKLVideoV25Turbo   = "kl-video-v2-5-turbo"
	ModelKLVideoV21Master  = "kl-video-v2-1-master"
	ModelKLVideoV21        = "kl-video-v2-1"
	ModelVDVideoQ3Pro      = "vd-video-q3-pro"
	ModelVDVideoQ3Turbo    = "vd-video-q3-turbo"
	requestPayloadKey      = "tencent_tokenhub_request_payload"
	defaultVideoDuration   = 5
)

var ModelList = []string{
	ModelHYVideo15,
	ModelYTVideo20,
	ModelYTVideoFX,
	ModelYTVideoHumanActor,
	ModelKLVideoV3,
	ModelKLVideoV26,
	ModelKLVideoV25Turbo,
	ModelKLVideoV21Master,
	ModelKLVideoV21,
	ModelVDVideoQ3Pro,
	ModelVDVideoQ3Turbo,
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
}

type taskResponse struct {
	ID          string          `json:"id,omitempty"`
	RequestID   string          `json:"request_id,omitempty"`
	Object      string          `json:"object,omitempty"`
	CreatedAt   int64           `json:"created_at,omitempty"`
	CompletedAt int64           `json:"completed_at,omitempty"`
	Status      string          `json:"status,omitempty"`
	Progress    int             `json:"progress,omitempty"`
	Data        taskData        `json:"data,omitempty"`
	Message     string          `json:"message,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
}

type taskData struct {
	URL string `json:"url,omitempty"`
}

func (a *TaskAdaptor) Init(*relaycommon.RelayInfo) {}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	var rawRequest map[string]json.RawMessage
	if err := common.UnmarshalBodyReusable(c, &rawRequest); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	modelName, _ := rawString(rawRequest["model"])
	if strings.TrimSpace(modelName) == "" {
		return service.TaskErrorWrapperLocal(errors.New("model is required"), "invalid_request", http.StatusBadRequest)
	}
	for _, key := range []string{"duration", "seconds", "duration_seconds", "billing_duration_seconds"} {
		if !validRawTaskDuration(rawRequest[key]) {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("%s must be between 1 and %d", key, relaycommon.MaxTaskDurationSeconds),
				"invalid_duration",
				http.StatusBadRequest,
			)
		}
	}

	metadata, err := decodeRawMetadata(rawRequest["metadata"])
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	for _, key := range []string{"duration", "seconds", "duration_seconds", "billing_duration_seconds"} {
		if !validRawTaskDuration(metadata[key]) {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("metadata.%s must be between 1 and %d", key, relaycommon.MaxTaskDurationSeconds),
				"invalid_duration",
				http.StatusBadRequest,
			)
		}
	}
	if modelName == ModelYTVideoHumanActor &&
		firstRawDuration(rawRequest, metadata, "duration", "seconds", "duration_seconds", "billing_duration_seconds") == 0 {
		return service.TaskErrorWrapperLocal(
			errors.New("billing_duration_seconds is required for yt-video-humanactor because billing follows the input audio duration"),
			"invalid_duration",
			http.StatusBadRequest,
		)
	}

	request := relaycommon.TaskSubmitReq{Model: modelName}
	request.Prompt, _ = rawString(rawRequest["prompt"])
	request.Size, _ = rawString(rawRequest["size"])
	request.Seconds, _ = rawString(rawRequest["seconds"])
	request.InputReference, _ = rawString(rawRequest["input_reference"])
	request.Image, _ = rawString(rawRequest["image"])
	if duration, ok := rawDuration(rawRequest["duration"]); ok {
		request.Duration = int(duration)
	}
	if len(metadata) > 0 {
		metadataData, err := common.Marshal(metadata)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		if err := common.Unmarshal(metadataData, &request.Metadata); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
	}

	info.Action = constant.TaskActionTextGenerate
	if hasImageInput(rawRequest, metadata) {
		info.Action = constant.TaskActionGenerate
	}
	c.Set(requestPayloadKey, rawRequest)
	c.Set("task_request", request)
	return nil
}

func validRawTaskDuration(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	duration, ok := rawDuration(raw)
	return ok && duration >= 1 && duration <= relaycommon.MaxTaskDurationSeconds
}

func rawDuration(raw json.RawMessage) (int64, bool) {
	var duration int64
	if err := common.Unmarshal(raw, &duration); err == nil {
		return duration, true
	}
	durationString, ok := rawString(raw)
	if !ok {
		return 0, false
	}
	if durationString == "" {
		return 0, true
	}
	duration, err := strconv.ParseInt(durationString, 10, 64)
	return duration, err == nil
}

func rawString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func decodeRawMetadata(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if metadataString, ok := rawString(raw); ok {
		if strings.TrimSpace(metadataString) == "" {
			return nil, nil
		}
		raw = json.RawMessage(metadataString)
	}
	var metadata map[string]json.RawMessage
	if err := common.Unmarshal(raw, &metadata); err != nil {
		return nil, errors.New("metadata must be a JSON object")
	}
	return metadata, nil
}

func hasImageInput(request, metadata map[string]json.RawMessage) bool {
	for _, values := range []map[string]json.RawMessage{request, metadata} {
		for _, key := range []string{"image", "images", "image_url", "image_base64", "input_reference"} {
			if raw := values[key]; len(raw) > 0 && string(raw) != "null" && string(raw) != `""` && string(raw) != "[]" {
				return true
			}
		}
	}
	return false
}

// EstimateBilling converts the documented TokenHub video price variants into
// positive multipliers over each model's base ModelPrice. Per-second models
// use the lowest documented resolution as their base price.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	value, ok := c.Get(requestPayloadKey)
	if !ok {
		return nil
	}
	request, ok := value.(map[string]json.RawMessage)
	if !ok {
		return nil
	}
	metadata, _ := decodeRawMetadata(request["metadata"])

	resolution := normalizeResolution(firstRawString(request, metadata, "resolution", "size"))
	if resolution == "" {
		resolution = defaultResolution(info.UpstreamModelName)
	}
	variant := tokenHubVideoVariantRatio(info.UpstreamModelName, resolution, request, metadata)

	switch info.UpstreamModelName {
	case ModelHYVideo15:
		return nil
	case ModelYTVideo20, ModelYTVideoFX:
		return map[string]float64{"resolution": variant}
	default:
		duration := firstRawDuration(request, metadata, "duration", "seconds", "duration_seconds", "billing_duration_seconds")
		if duration == 0 {
			if info.UpstreamModelName == ModelYTVideoHumanActor {
				duration = 60
			} else {
				duration = defaultVideoDuration
			}
		}
		return map[string]float64{
			"seconds":    float64(duration),
			"resolution": variant,
		}
	}
}

func firstRawString(request, metadata map[string]json.RawMessage, keys ...string) string {
	for _, values := range []map[string]json.RawMessage{request, metadata} {
		for _, key := range keys {
			if value, ok := rawString(values[key]); ok && strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}

func firstRawDuration(request, metadata map[string]json.RawMessage, keys ...string) int64 {
	for _, values := range []map[string]json.RawMessage{request, metadata} {
		for _, key := range keys {
			if duration, ok := rawDuration(values[key]); ok && duration > 0 {
				return duration
			}
		}
	}
	return 0
}

func normalizeResolution(resolution string) string {
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	resolution = strings.ReplaceAll(resolution, " ", "")
	switch resolution {
	case "3840x2160", "2160x3840", "2160p", "4k":
		return "4k"
	case "1920x1080", "1080x1920", "1080p":
		return "1080p"
	case "1280x720", "720x1280", "720p":
		return "720p"
	case "960x540", "540x960", "540p":
		return "540p"
	case "640x360", "360x640", "360p":
		return "360p"
	default:
		return resolution
	}
}

func defaultResolution(modelName string) string {
	switch modelName {
	case ModelYTVideo20:
		return "480p"
	case ModelYTVideoFX:
		return "360p"
	case ModelYTVideoHumanActor:
		return "1080p"
	case ModelVDVideoQ3Pro, ModelVDVideoQ3Turbo:
		return "720p"
	default:
		return "720p"
	}
}

func tokenHubVideoVariantRatio(modelName, resolution string, request, metadata map[string]json.RawMessage) float64 {
	hasAudio := firstRawBool(request, metadata, "sound", "audio", "generate_audio", "with_audio")
	hasSpecifiedVoice := firstRawString(request, metadata, "voice_id", "voice", "voice_type") != ""
	switch modelName {
	case ModelYTVideo20:
		if resolution == "720p" || resolution == "1080p" {
			return 2.5
		}
	case ModelYTVideoFX:
		if resolution == "720p" {
			return 2
		}
	case ModelYTVideoHumanActor:
		if resolution == "1080p" {
			return 2
		}
	case ModelKLVideoV3:
		switch resolution {
		case "4k":
			return 5
		case "1080p":
			if hasAudio {
				return 2
			}
			return 4.0 / 3.0
		default:
			if hasAudio {
				return 1.5
			}
		}
	case ModelKLVideoV26:
		switch resolution {
		case "4k":
			return 10
		case "1080p":
			if hasAudio && hasSpecifiedVoice {
				return 4
			}
			if hasAudio {
				return 10.0 / 3.0
			}
			return 5.0 / 3.0
		}
	case ModelKLVideoV25Turbo:
		if resolution == "1080p" {
			return 5.0 / 3.0
		}
	case ModelKLVideoV21:
		if resolution == "1080p" {
			return 1.75
		}
	case ModelVDVideoQ3Pro:
		switch resolution {
		case "720p":
			return 20.0 / 9.0
		case "1080p":
			return 8.0 / 3.0
		}
	case ModelVDVideoQ3Turbo:
		switch resolution {
		case "720p":
			return 12.0 / 7.0
		case "1080p":
			return 13.0 / 7.0
		}
	}
	return 1
}

func firstRawBool(request, metadata map[string]json.RawMessage, keys ...string) bool {
	for _, values := range []map[string]json.RawMessage{request, metadata} {
		for _, key := range keys {
			raw := values[key]
			if len(raw) == 0 || string(raw) == "null" {
				continue
			}
			var enabled bool
			if err := common.Unmarshal(raw, &enabled); err == nil {
				return enabled
			}
			if value, ok := rawString(raw); ok {
				return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
			}
		}
	}
	return false
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if !IsVideoModel(info.UpstreamModelName) {
		return "", fmt.Errorf("unsupported Tencent TokenHub video model: %s", info.UpstreamModelName)
	}
	return strings.TrimRight(info.ChannelBaseUrl, "/") + "/v1/api/video/submit", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	value, ok := c.Get(requestPayloadKey)
	if !ok {
		return nil, errors.New("Tencent TokenHub request payload not found in context")
	}
	request, ok := value.(map[string]json.RawMessage)
	if !ok {
		return nil, errors.New("invalid Tencent TokenHub request payload")
	}
	metadata, err := decodeRawMetadata(request["metadata"])
	if err != nil {
		return nil, err
	}
	payload := make(map[string]json.RawMessage, len(request)+len(metadata))
	for key, value := range metadata {
		if key == "model" || key == "id" {
			continue
		}
		payload[key] = value
	}
	for key, value := range request {
		if key == "metadata" || key == "model" || key == "id" {
			continue
		}
		payload[key] = value
	}
	modelData, err := common.Marshal(info.UpstreamModelName)
	if err != nil {
		return nil, err
	}
	payload["model"] = modelData
	if _, ok := payload["resolution"]; !ok {
		if size, ok := payload["size"]; ok {
			payload["resolution"] = size
		}
	}
	delete(payload, "size")
	if _, ok := payload["duration"]; !ok {
		if seconds, exists := payload["seconds"]; exists {
			duration, valid := rawDuration(seconds)
			if !valid || duration < 1 || duration > relaycommon.MaxTaskDurationSeconds {
				return nil, fmt.Errorf("seconds must be between 1 and %d", relaycommon.MaxTaskDurationSeconds)
			}
			payload["duration"], err = common.Marshal(duration)
			if err != nil {
				return nil, err
			}
		}
	}
	delete(payload, "seconds")
	delete(payload, "billing_duration_seconds")

	if _, ok := payload["image"]; !ok {
		if inputReference, ok := payload["input_reference"]; ok {
			payload["image"], err = normalizeTokenHubImage(inputReference)
			if err != nil {
				return nil, err
			}
		}
	} else {
		payload["image"], err = normalizeTokenHubImage(payload["image"])
		if err != nil {
			return nil, err
		}
	}
	delete(payload, "input_reference")
	if images, ok := payload["images"]; ok {
		var items []json.RawMessage
		if err := common.Unmarshal(images, &items); err == nil {
			for index, item := range items {
				items[index], err = normalizeTokenHubImage(item)
				if err != nil {
					return nil, err
				}
			}
			payload["images"], err = common.Marshal(items)
			if err != nil {
				return nil, err
			}
		}
	}

	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func normalizeTokenHubImage(raw json.RawMessage) (json.RawMessage, error) {
	image, ok := rawString(raw)
	if !ok {
		return raw, nil
	}
	key := "url"
	image = strings.TrimSpace(image)
	if strings.HasPrefix(image, "data:") {
		if comma := strings.IndexByte(image, ','); comma >= 0 {
			key = "base64"
			image = image[comma+1:]
		}
	}
	return common.Marshal(map[string]string{key: image})
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *taskdto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	service.CloseResponseBodyGracefully(resp)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	var upstream taskResponse
	if err := common.Unmarshal(body, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if upstream.ID == "" {
		return "", nil, service.TaskErrorWrapperLocal(
			fmt.Errorf("Tencent TokenHub response is missing id: %s", body),
			"invalid_response",
			http.StatusBadGateway,
		)
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = info.PublicTaskID
	openAIVideo.TaskID = info.PublicTaskID
	openAIVideo.Model = info.OriginModelName
	openAIVideo.CreatedAt = upstream.CreatedAt
	if openAIVideo.CreatedAt == 0 {
		openAIVideo.CreatedAt = time.Now().Unix()
	}
	openAIVideo.Status = dto.VideoStatusQueued
	c.JSON(http.StatusOK, openAIVideo)
	return upstream.ID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, errors.New("invalid task_id")
	}
	modelName, ok := body["model"].(string)
	if !ok || modelName == "" {
		return nil, errors.New("invalid model")
	}
	payload, err := common.Marshal(map[string]string{"model": modelName, "id": taskID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/api/video/query", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	var response taskResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	result := &relaycommon.TaskInfo{}
	switch response.Status {
	case "queued", "pending":
		result.Status = model.TaskStatusQueued
	case "processing", "in_progress":
		result.Status = model.TaskStatusInProgress
	case "completed":
		result.Status = model.TaskStatusSuccess
		result.Url = response.Data.URL
	case "failed", "cancelled":
		result.Status = model.TaskStatusFailure
		result.Reason = taskErrorMessage(response)
	default:
		return nil, fmt.Errorf("unknown Tencent TokenHub task status: %s", response.Status)
	}
	if response.Progress > 0 {
		result.Progress = fmt.Sprintf("%d%%", min(response.Progress, 100))
	}
	return result, nil
}

func taskErrorMessage(response taskResponse) string {
	if response.Message != "" {
		return response.Message
	}
	if len(response.Error) > 0 {
		var detail struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}
		if err := common.Unmarshal(response.Error, &detail); err == nil {
			if detail.Message != "" {
				return detail.Message
			}
			if detail.Code != "" {
				return detail.Code
			}
		}
	}
	return "Tencent TokenHub task failed"
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var response taskResponse
	if len(task.Data) > 0 {
		if err := common.Unmarshal(task.Data, &response); err != nil {
			return nil, err
		}
	}
	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.TaskID = task.TaskID
	video.Model = task.Properties.OriginModelName
	video.Status = task.Status.ToVideoStatus()
	video.CreatedAt = task.CreatedAt
	video.CompletedAt = task.FinishTime
	video.SetProgressStr(task.Progress)
	if response.Data.URL != "" {
		video.SetMetadata("url", response.Data.URL)
	} else if resultURL := task.GetResultURL(); resultURL != "" {
		video.SetMetadata("url", resultURL)
	}
	if task.Status == model.TaskStatusFailure {
		video.Error = &dto.OpenAIVideoError{Message: taskErrorMessage(response), Code: "task_failed"}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func IsVideoModel(modelName string) bool {
	for _, supportedModel := range ModelList {
		if modelName == supportedModel {
			return true
		}
	}
	return false
}
