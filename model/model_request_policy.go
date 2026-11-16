package model

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hostreasoning "github.com/QuantumNous/new-api/setting/reasoning"
)

// Policies belong to public model names, independently of catalog metadata and
// upstream aliases. Existing option replication also distributes these settings.
const ModelRequestPolicyOptionPrefix = "ModelRequestPolicy."

type ModelRequestPolicy struct {
	Model                 string                         `json:"model"`
	ParameterCapabilities *dto.ParameterCapabilityConfig `json:"parameter_capabilities,omitempty"`
	ParamOverride         map[string]any                 `json:"param_override,omitempty"`
}

var modelRequestPolicyCache sync.Map

type cachedModelRequestPolicy struct {
	value  string
	policy *ModelRequestPolicy
}

func modelRequestPolicyKey(model string) string {
	return fmt.Sprintf("%s%x", ModelRequestPolicyOptionPrefix, sha256.Sum256([]byte(model)))
}

func ParseModelRequestPolicy(value string) (*ModelRequestPolicy, error) {
	var policy ModelRequestPolicy
	if err := common.UnmarshalJsonStr(value, &policy); err != nil {
		return nil, err
	}
	if policy.Model == "" || len(policy.Model) > 255 || strings.TrimSpace(policy.Model) != policy.Model || strings.ContainsAny(policy.Model, "\x00\r\n") {
		return nil, fmt.Errorf("invalid model name")
	}
	if err := policy.ParameterCapabilities.Validate(); err != nil {
		return nil, err
	}
	if value, exists := policy.ParamOverride["operations"]; exists {
		operations, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("parameter override operations must be an array")
		}
		for _, value := range operations {
			operation, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("parameter override operation must be an object")
			}
			mode, ok := operation["mode"].(string)
			if !ok || strings.TrimSpace(mode) == "" {
				return nil, fmt.Errorf("parameter override operation mode is required")
			}
		}
	}
	return &policy, nil
}

// GetModelRequestPolicy reads an immutable snapshot. Matching uses the public
// request model first, then its base name for request modifiers.
func GetModelRequestPolicy(model string) *ModelRequestPolicy {
	names := []string{model}
	if base := hostreasoning.BaseModelName(model); base != model && base != "" {
		names = append(names, base)
	}
	for _, name := range names {
		key := modelRequestPolicyKey(name)
		common.OptionMapRWMutex.RLock()
		value := common.OptionMap[key]
		common.OptionMapRWMutex.RUnlock()
		if value == "" {
			continue
		}
		if cached, ok := modelRequestPolicyCache.Load(key); ok && cached.(cachedModelRequestPolicy).value == value {
			return cached.(cachedModelRequestPolicy).policy
		}
		policy, err := ParseModelRequestPolicy(value)
		if err != nil || policy.Model != name {
			common.SysError("invalid model request policy: " + name)
			return nil
		}
		modelRequestPolicyCache.Store(key, cachedModelRequestPolicy{value: value, policy: policy})
		return policy
	}
	return nil
}

func SaveModelRequestPolicy(policy ModelRequestPolicy) error {
	value, err := common.Marshal(policy)
	if err != nil {
		return err
	}
	return UpdateOption(modelRequestPolicyKey(policy.Model), string(value))
}

// EffectiveModelParameterCapabilities resolves each scope against its own model
// namespace, then applies channel fields over model defaults without mutation.
func EffectiveModelParameterCapabilities(requestModel, upstreamModel string, channel *dto.ParameterCapabilityConfig) *dto.ParameterCapabilityConfig {
	policy := GetModelRequestPolicy(requestModel)
	if policy == nil || policy.ParameterCapabilities == nil {
		return channel
	}
	upstreamModel = hostreasoning.BaseModelName(upstreamModel)
	merged := &dto.ParameterCapabilityConfig{
		Defaults: policy.ParameterCapabilities.Resolve(policy.Model),
		Rules: []dto.ModelParameterCapabilityRule{{
			Selector:   dto.ParameterCapabilitySelector{Type: dto.ParameterCapabilitySelectorExact, Value: upstreamModel},
			Parameters: channel.Resolve(upstreamModel),
		}},
	}
	return &dto.ParameterCapabilityConfig{Defaults: merged.Resolve(upstreamModel)}
}

// EffectiveModelParamOverride merges legacy fields and ordered operations.
// Cross-layer duplicates retain the channel occurrence, preserving its priority.
// Repetition within a layer remains intentional; overlap is matched one-for-one.
func EffectiveModelParamOverride(requestModel string, channel map[string]any) map[string]any {
	policy := GetModelRequestPolicy(requestModel)
	if policy == nil || len(policy.ParamOverride) == 0 {
		return channel
	}
	if len(channel) == 0 {
		return policy.ParamOverride
	}
	layers := [2][]any{}
	for i, layer := range []map[string]any{policy.ParamOverride, channel} {
		_, hasOperations := layer["operations"]
		keys := slices.Sorted(maps.Keys(layer))
		for _, key := range keys {
			if hasOperations && strings.EqualFold(strings.TrimSpace(key), "operations") {
				continue
			}
			path := strings.NewReplacer("\\", "\\\\", ".", "\\.", "*", "\\*", "?", "\\?").Replace(key)
			layers[i] = append(layers[i], map[string]any{"mode": "set", "path": path, "value": layer[key]})
		}
		if value, exists := layer["operations"]; exists {
			layerOperations, ok := value.([]any)
			if !ok {
				// Preserve the existing runtime error for malformed channel configs.
				return map[string]any{"operations": value}
			}
			layers[i] = append(layers[i], layerOperations...)
		}
	}
	channelCopies := make(map[string]int)
	for _, operation := range layers[1] {
		if key, ok := parameterOverrideOperationKey(operation); ok {
			channelCopies[key]++
		}
	}
	duplicates := make([]bool, len(layers[0]))
	for i := len(layers[0]) - 1; i >= 0; i-- {
		key, ok := parameterOverrideOperationKey(layers[0][i])
		if ok && channelCopies[key] > 0 {
			duplicates[i] = true
			channelCopies[key]--
		}
	}
	operations := make([]any, 0, len(layers[0])+len(layers[1]))
	for i, operation := range layers[0] {
		if !duplicates[i] {
			operations = append(operations, operation)
		}
	}
	operations = append(operations, layers[1]...)
	return map[string]any{"operations": operations}
}

// parameterOverrideOperationKey ignores display descriptions and equivalent
// optional defaults. Values and condition order remain significant. Cloning
// avoids changing persisted configuration or snapshots shared by requests.
func parameterOverrideOperationKey(value any) (string, bool) {
	operation, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	if _, ok := operation["mode"].(string); !ok {
		return "", false
	}
	identity := maps.Clone(operation)
	delete(identity, "description")
	for _, key := range []string{"path", "from", "to", "logic"} {
		if value, ok := identity[key].(string); ok && value == "" {
			delete(identity, key)
		}
	}
	if value, ok := identity["keep_origin"].(bool); ok && !value {
		delete(identity, "keep_origin")
	}
	if identity["value"] == nil {
		delete(identity, "value")
	}
	if logic, ok := identity["logic"].(string); ok {
		switch strings.ToUpper(logic) {
		case "OR":
			delete(identity, "logic")
		case "AND":
			identity["logic"] = "AND"
		}
	}
	if conditions, ok := identity["conditions"].([]any); ok {
		if len(conditions) == 0 {
			delete(identity, "conditions")
		} else {
			normalized := make([]any, 0, len(conditions))
			for _, value := range conditions {
				condition, ok := value.(map[string]any)
				if !ok {
					return "", false
				}
				condition = maps.Clone(condition)
				for _, key := range []string{"invert", "pass_missing_key"} {
					if value, ok := condition[key].(bool); ok && !value {
						delete(condition, key)
					}
				}
				normalized = append(normalized, condition)
			}
			identity["conditions"] = normalized
		}
	}
	encoded, err := common.Marshal(identity)
	return string(encoded), err == nil
}
