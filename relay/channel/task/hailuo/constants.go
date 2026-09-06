package hailuo

const (
	ChannelName = "hailuo-video"
)

var ModelList = []string{
	"MiniMax-H3",
	"MiniMax-Hailuo-2.3",
	"MiniMax-Hailuo-2.3-Fast",
	"MiniMax-Hailuo-02",
	"T2V-01-Director",
	"T2V-01",
	"I2V-01-Director",
	"I2V-01-live",
	"I2V-01",
	"S2V-01",
}

const (
	H3Model = "MiniMax-H3"

	TextToVideoEndpoint = "/v1/video_generation"
	QueryTaskEndpoint   = "/v1/query/video_generation"
	H3VideoEndpoint     = "/v2/video_generation"
	H3QueryTaskEndpoint = "/v2/query/video_generation"
)

const (
	StatusSuccess    = 0
	StatusRateLimit  = 1002
	StatusAuthFailed = 1004
	StatusNoBalance  = 1008
	StatusSensitive  = 1026
	StatusParamError = 2013
	StatusInvalidKey = 2049
)

const (
	TaskStatusPreparing  = "Preparing"
	TaskStatusQueueing   = "Queueing"
	TaskStatusProcessing = "Processing"
	TaskStatusSuccess    = "Success"
	TaskStatusFailed     = "Fail"
	H3TaskStatusQueued   = "queued"
	H3TaskStatusRunning  = "running"
	H3TaskStatusSuccess  = "succeeded"
	H3TaskStatusFailed   = "failed"
	H3TaskStatusCanceled = "cancelled"
)

const (
	Resolution512P  = "512P"
	Resolution720P  = "720P"
	Resolution768P  = "768P"
	Resolution1080P = "1080P"
	Resolution2K    = "2K"
)

const (
	DefaultDuration   = 6
	DefaultResolution = Resolution720P
)

const (
	H3MinDuration         = 4
	H3MaxDuration         = 15
	H3DefaultDuration     = 5
	H3MaxFrameImages      = 2
	H3MaxReferenceImages  = 9
	H3MaxReferenceVideos  = 3
	H3MaxReferenceAudios  = 3
	H3MaxPromptCharacters = 7000
	H3FreeInputImages     = 5
)

const (
	h3BasePriceRMBPerSecond = 0.5
	h3PriceRMBPer2KSecond   = 0.8
	h3PriceRMBPerExtraImage = 0.2
)

var H3Ratios = []string{"adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}
