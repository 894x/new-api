package sim

import "time"

// Candidate is the narrow routing view exposed to a policy.
type Candidate struct {
	ID        string
	ChannelID int
	Priority  int64
	Weight    uint
}

// Sample is delivered when a simulated request completes.
type Sample struct {
	CandidateID string
	ArrivedAt   time.Duration
	CompletedAt time.Duration
	TTFT        time.Duration
	TPOT        time.Duration
	Success     bool
	Failure     FailureKind
}

// Decision is a policy's routing result.
type Decision struct {
	CandidateID          string
	DominantCandidate    string
	Probe                bool
	DegradedCandidates   []string
	EmergencyCandidates  []string
	UnverifiedCandidates []string
}

// Policy is deliberately small so the production controller can be adapted
// without coupling the simulator to its internal state representation.
type Policy interface {
	Select(now time.Duration, candidates []Candidate) Decision
	Observe(sample Sample)
}

type SLO struct {
	TTFT time.Duration
	TPOT time.Duration
}

type Phase struct {
	Start           time.Duration
	TTFT            time.Duration
	TPOT            time.Duration
	TTFTJitter      time.Duration
	TPOTJitter      time.Duration
	LongTailRate    float64
	LongTailDelay   time.Duration
	HTTP429Rate     float64
	HTTP503Rate     float64
	HardFailureRate float64
}

type Channel struct {
	ID          string
	ChannelID   int
	Priority    int64
	Weight      uint
	Concurrency int
	Timeline    []Phase
}

type Scenario struct {
	Name         string
	Seed         int64
	Arrivals     []time.Duration
	OutputTokens int
	Channels     []Channel
	SLO          SLO
	Fault        FaultSpec
}

type FaultSpec struct {
	At                time.Duration
	BadChannels       []string
	MitigationWindow  int
	MitigatedBadShare float64
}

type FailureKind string

const (
	FailureNone    FailureKind = ""
	FailureHTTP429 FailureKind = "http_429"
	FailureHTTP503 FailureKind = "http_503"
	FailureHard    FailureKind = "hard_failure"
)

type RequestResult struct {
	ArrivedAt            time.Duration
	StartedAt            time.Duration
	CompletedAt          time.Duration
	ChannelID            string
	DominantCandidate    string
	TTFT                 time.Duration
	TPOT                 time.Duration
	Success              bool
	Failure              FailureKind
	Probe                bool
	DegradedCandidates   []string
	EmergencyCandidates  []string
	UnverifiedCandidates []string
}

type Metrics struct {
	P95TTFT                          time.Duration
	P95TPOT                          time.Duration
	TotalRequests                    int
	Successes                        int
	SuccessRate                      float64
	ThroughputPerSecond              float64
	SLOViolationRate                 float64
	SLOViolationArea                 float64
	BadChannelExposureAfterFault     int
	BadChannelExposureAfterDetection int
	DetectionDelay                   time.Duration
	DetectionObservations            int
	MitigationDelay                  time.Duration
	RouteReversals                   int
	ProbeCost                        float64
}

type Result struct {
	Scenario string
	Requests []RequestResult
	Metrics  Metrics
}
