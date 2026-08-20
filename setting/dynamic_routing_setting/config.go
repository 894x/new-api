package dynamic_routing_setting

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	"github.com/QuantumNous/new-api/setting/config"
)

// DynamicRoutingSetting is the persisted admin configuration. Duration values
// use seconds so they can be edited through the existing flat option API.
type DynamicRoutingSetting struct {
	Enabled                    bool    `json:"enabled"`
	MaxSamples                 int     `json:"max_samples"`
	MaxAgeSeconds              int     `json:"max_age_seconds"`
	MinSamples                 int     `json:"min_samples"`
	ProbeFraction              float64 `json:"probe_fraction"`
	DegradationThreshold       float64 `json:"degradation_threshold"`
	RecoveryThreshold          float64 `json:"recovery_threshold"`
	CriticalThreshold          float64 `json:"critical_threshold"`
	CandidateAdvantage         float64 `json:"candidate_advantage"`
	Aggressiveness             float64 `json:"aggressiveness"`
	RecoveryStep               float64 `json:"recovery_step"`
	CooldownSeconds            int     `json:"cooldown_seconds"`
	HardFailureThreshold       int     `json:"hard_failure_threshold"`
	HardFailureCooldownSeconds int     `json:"hard_failure_cooldown_seconds"`
}

type Snapshot struct {
	Version uint64
	Config  dynamicrouting.Config
}

const OptionPrefix = "dynamic_routing_setting."

var dynamicRoutingSetting = DynamicRoutingSetting{
	Enabled:                    false,
	MaxSamples:                 60,
	MaxAgeSeconds:              90,
	MinSamples:                 3,
	ProbeFraction:              0.015,
	DegradationThreshold:       1.3,
	RecoveryThreshold:          1.1,
	CriticalThreshold:          1.9,
	CandidateAdvantage:         1.1,
	Aggressiveness:             0.90,
	RecoveryStep:               0.02,
	CooldownSeconds:            3,
	HardFailureThreshold:       1,
	HardFailureCooldownSeconds: 30,
}

var (
	settingMu         sync.RWMutex
	publishedSnapshot atomic.Pointer[Snapshot]
	snapshotVersion   atomic.Uint64
)

func init() {
	config.GlobalConfig.Register("dynamic_routing_setting", &dynamicRoutingSetting)
	if err := UpdateAndSync(); err != nil {
		panic(err)
	}
}

func GetSetting() DynamicRoutingSetting {
	settingMu.RLock()
	defer settingMu.RUnlock()
	return dynamicRoutingSetting
}

func GetSnapshot() Snapshot {
	return *publishedSnapshot.Load()
}

// UpdateAndSync validates the mutable persisted setting and atomically
// publishes an immutable runtime snapshot. Invalid updates leave the last good
// snapshot active.
func UpdateAndSync() error {
	settingMu.Lock()
	defer settingMu.Unlock()
	return publishSettingLocked(dynamicRoutingSetting)
}

// ReplaceAndSync validates and publishes one complete setting. The mutable
// persisted view and immutable runtime snapshot change together only after the
// entire candidate is known to be valid.
func ReplaceAndSync(setting DynamicRoutingSetting) error {
	settingMu.Lock()
	defer settingMu.Unlock()

	if err := validateSetting(setting); err != nil {
		return err
	}
	if setting == dynamicRoutingSetting {
		return nil
	}
	dynamicRoutingSetting = setting
	return publishSettingLocked(setting)
}

func Validate(setting DynamicRoutingSetting) error {
	return validateSetting(setting)
}

func publishSettingLocked(setting DynamicRoutingSetting) error {
	runtimeConfig, err := validatedRuntimeConfig(setting)
	if err != nil {
		return err
	}

	publishedSnapshot.Store(&Snapshot{
		Version: snapshotVersion.Add(1),
		Config:  runtimeConfig,
	})
	return nil
}

func validateSetting(setting DynamicRoutingSetting) error {
	_, err := validatedRuntimeConfig(setting)
	return err
}

func validatedRuntimeConfig(setting DynamicRoutingSetting) (dynamicrouting.Config, error) {
	// The core accepts zero for several fields as a backwards-compatible
	// request for defaults. Persisted admin settings must be explicit so the
	// stored values, OptionMap, and published runtime snapshot cannot disagree.
	if setting.MinSamples <= 0 {
		return dynamicrouting.Config{}, errors.New("min samples must be positive")
	}
	if setting.DegradationThreshold <= 1 {
		return dynamicrouting.Config{}, errors.New("degradation threshold must be greater than 1")
	}
	if setting.RecoveryThreshold <= 1 {
		return dynamicrouting.Config{}, errors.New("recovery threshold must be greater than 1")
	}
	if setting.CriticalThreshold <= 1 {
		return dynamicrouting.Config{}, errors.New("critical threshold must be greater than 1")
	}
	if setting.CandidateAdvantage <= 1 {
		return dynamicrouting.Config{}, errors.New("candidate advantage must be greater than 1")
	}
	if setting.Aggressiveness <= 0 {
		return dynamicrouting.Config{}, errors.New("aggressiveness must be positive")
	}
	if setting.RecoveryStep <= 0 {
		return dynamicrouting.Config{}, errors.New("recovery step must be positive")
	}
	if setting.HardFailureThreshold <= 0 {
		return dynamicrouting.Config{}, errors.New("hard failure threshold must be positive")
	}
	if setting.HardFailureCooldownSeconds <= 0 {
		return dynamicrouting.Config{}, errors.New("hard failure cooldown must be positive")
	}

	runtimeConfig, err := runtimeConfigFromSetting(setting)
	if err != nil {
		return dynamicrouting.Config{}, err
	}
	if _, err := dynamicrouting.NewController(runtimeConfig); err != nil {
		return dynamicrouting.Config{}, err
	}
	return runtimeConfig, nil
}

func ToOptionValues(setting DynamicRoutingSetting) map[string]string {
	return map[string]string{
		OptionPrefix + "enabled":                       strconv.FormatBool(setting.Enabled),
		OptionPrefix + "max_samples":                   strconv.Itoa(setting.MaxSamples),
		OptionPrefix + "max_age_seconds":               strconv.Itoa(setting.MaxAgeSeconds),
		OptionPrefix + "min_samples":                   strconv.Itoa(setting.MinSamples),
		OptionPrefix + "probe_fraction":                strconv.FormatFloat(setting.ProbeFraction, 'g', -1, 64),
		OptionPrefix + "degradation_threshold":         strconv.FormatFloat(setting.DegradationThreshold, 'g', -1, 64),
		OptionPrefix + "recovery_threshold":            strconv.FormatFloat(setting.RecoveryThreshold, 'g', -1, 64),
		OptionPrefix + "critical_threshold":            strconv.FormatFloat(setting.CriticalThreshold, 'g', -1, 64),
		OptionPrefix + "candidate_advantage":           strconv.FormatFloat(setting.CandidateAdvantage, 'g', -1, 64),
		OptionPrefix + "aggressiveness":                strconv.FormatFloat(setting.Aggressiveness, 'g', -1, 64),
		OptionPrefix + "recovery_step":                 strconv.FormatFloat(setting.RecoveryStep, 'g', -1, 64),
		OptionPrefix + "cooldown_seconds":              strconv.Itoa(setting.CooldownSeconds),
		OptionPrefix + "hard_failure_threshold":        strconv.Itoa(setting.HardFailureThreshold),
		OptionPrefix + "hard_failure_cooldown_seconds": strconv.Itoa(setting.HardFailureCooldownSeconds),
	}
}

// MergeOptionValues builds and validates one candidate from flat persisted
// values without publishing intermediate per-field states.
func MergeOptionValues(base DynamicRoutingSetting, values map[string]string) (DynamicRoutingSetting, error) {
	updates := make(map[string]string, len(values))
	for key, value := range values {
		if !strings.HasPrefix(key, OptionPrefix) {
			return DynamicRoutingSetting{}, fmt.Errorf("unknown dynamic routing option %q", key)
		}
		updates[strings.TrimPrefix(key, OptionPrefix)] = value
	}
	if err := config.UpdateConfigFromMap(&base, updates); err != nil {
		return DynamicRoutingSetting{}, err
	}
	if err := validateSetting(base); err != nil {
		return DynamicRoutingSetting{}, err
	}
	return base, nil
}

func runtimeConfigFromSetting(setting DynamicRoutingSetting) (dynamicrouting.Config, error) {
	const maxDurationSeconds = int64((1<<63 - 1) / int64(time.Second))
	if int64(setting.MaxAgeSeconds) > maxDurationSeconds || int64(setting.CooldownSeconds) > maxDurationSeconds || int64(setting.HardFailureCooldownSeconds) > maxDurationSeconds {
		return dynamicrouting.Config{}, errors.New("dynamic routing duration exceeds time.Duration")
	}
	return dynamicrouting.Config{
		Enabled:              setting.Enabled,
		MaxSamples:           setting.MaxSamples,
		MaxAge:               time.Duration(setting.MaxAgeSeconds) * time.Second,
		MinSamples:           setting.MinSamples,
		ProbeFraction:        setting.ProbeFraction,
		DegradationThreshold: setting.DegradationThreshold,
		RecoveryThreshold:    setting.RecoveryThreshold,
		CriticalThreshold:    setting.CriticalThreshold,
		CandidateAdvantage:   setting.CandidateAdvantage,
		Aggressiveness:       setting.Aggressiveness,
		RecoveryStep:         setting.RecoveryStep,
		Cooldown:             time.Duration(setting.CooldownSeconds) * time.Second,
		HardFailureThreshold: setting.HardFailureThreshold,
		HardFailureCooldown:  time.Duration(setting.HardFailureCooldownSeconds) * time.Second,
	}, nil
}
