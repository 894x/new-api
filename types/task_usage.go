package types

const (
	TaskUsageKindVideoDuration = "video_duration"
	TaskUsageUnitSecond        = "second"
)

// TaskUsage describes provider-reported usage without converting it into the
// synthetic token units used internally by task settlement.
type TaskUsage struct {
	Kind        string  `json:"kind"`
	Unit        string  `json:"unit"`
	Input       float64 `json:"input"`
	Output      float64 `json:"output"`
	Total       float64 `json:"total"`
	InputImages *int    `json:"input_images,omitempty"`
}
