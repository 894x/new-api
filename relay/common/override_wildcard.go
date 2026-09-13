package common

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/relayparam"
	"github.com/tidwall/gjson"
)

// wildcardBinding binds a source/path match to targets by capture order, and
// to conditions by shared array/object scope. Unrelated collections never zip.
type wildcardBinding struct {
	anchor   []string
	concrete []string
}

func (b wildcardBinding) bind(path string, target bool) (string, error) {
	if !target && !relayparam.HasJSONPathWildcard(path) {
		return path, nil
	}
	parts := relayparam.JSONPathSegments(path)
	original := slices.Clone(parts)
	var captures []string
	for i, segment := range b.anchor {
		if segment == "*" {
			captures = append(captures, b.concrete[i])
		}
	}
	count := 0
	for i, segment := range parts {
		if segment != "*" {
			continue
		}
		if target {
			if count >= len(captures) {
				return "", fmt.Errorf("target wildcard count must match source: %s", path)
			}
			parts[i] = captures[count]
		} else {
			if i >= len(b.anchor) || !slices.Equal(original[:i+1], b.anchor[:i+1]) {
				return "", fmt.Errorf("unbound condition wildcard in %s; use the operation's source/path scope", path)
			}
			parts[i] = b.concrete[i]
		}
		count++
	}
	if target && count != len(captures) {
		return "", fmt.Errorf("target wildcard count must match source: %s", path)
	}
	return strings.Join(parts, "."), nil
}

// expandWildcardOperation fixes matches and conditions before any item is
// changed. The outer operations list still executes sequentially.
func expandWildcardOperation(data []byte, contextJSON string, op ParamOperation) ([]ParamOperation, bool, error) {
	anchor := ""
	transfer := op.Mode == "move" || op.Mode == "copy"
	if transfer {
		anchor = op.From
	} else if isPathBasedOperation(op.Mode) {
		anchor = op.Path
	}
	if !relayparam.HasJSONPathWildcard(anchor) {
		if transfer && relayparam.HasJSONPathWildcard(op.To) {
			return nil, false, fmt.Errorf("target wildcard count must match source: %s", op.To)
		}
		for _, condition := range op.Conditions {
			for _, path := range []string{condition.Path, condition.ValuePath} {
				if relayparam.HasJSONPathWildcard(path) {
					return nil, false, fmt.Errorf("unbound condition wildcard in %s; an operation source/path wildcard is required", path)
				}
			}
		}
		return nil, false, nil
	}
	anchorParts := relayparam.JSONPathSegments(anchor)
	// Validate even with an empty collection, before evaluating OR/AND.
	validation := wildcardBinding{anchor: anchorParts, concrete: anchorParts}
	for _, condition := range op.Conditions {
		for _, path := range []string{condition.Path, condition.ValuePath} {
			if _, err := validation.bind(path, false); err != nil {
				return nil, false, err
			}
		}
	}
	if transfer {
		if _, err := validation.bind(op.To, true); err != nil {
			return nil, false, err
		}
	}
	paths, err := relayparam.ResolveJSONPaths(data, anchor, !transfer)
	if err != nil {
		return nil, true, err
	}
	matches := make([]ParamOperation, 0, len(paths))
	for _, path := range paths {
		binding := wildcardBinding{anchor: anchorParts, concrete: relayparam.JSONPathSegments(path)}
		bound := op
		if transfer {
			bound.From = path
			bound.To, err = binding.bind(op.To, true)
			if err != nil {
				return nil, true, err
			}
		} else {
			bound.Path = path
		}
		conditions := make([]ConditionOperation, len(op.Conditions))
		for i, condition := range op.Conditions {
			condition.Path, err = binding.bind(condition.Path, false)
			if err != nil {
				return nil, true, err
			}
			condition.ValuePath, err = binding.bind(condition.ValuePath, false)
			if err != nil {
				return nil, true, err
			}
			conditions[i] = condition
		}
		ok, err := checkConditions(data, contextJSON, conditions, op.Logic)
		if err != nil {
			return nil, true, err
		}
		if !ok {
			continue
		}
		if transfer {
			bound.To, err = resolveWildcardTarget(data, bound.To)
			if err != nil {
				return nil, true, err
			}
		}
		bound.Conditions = nil
		matches = append(matches, bound)
	}
	if transfer {
		if err := validateWildcardDestinations(matches); err != nil {
			return nil, true, err
		}
	}
	if op.Mode == "delete" || op.Mode == "move" {
		// Resolver visits arrays in index order. Delete from the end so original
		// indices remain valid, including nested arrays and filtered matches.
		slices.Reverse(matches)
	}
	return matches, true, nil
}

// Canonicalize array indices before checking overlap, so aliases such as 00
// cannot bypass conflict detection. Missing containers follow sjson's numeric
// index inference; existing objects keep numeric field names unchanged.
func resolveWildcardTarget(data []byte, path string) (string, error) {
	parts := relayparam.JSONPathSegments(path)
	node := gjson.ParseBytes(data)
	for i, part := range parts {
		if node.IsArray() || !node.Exists() {
			index, err := strconv.Atoi(part)
			if err != nil && node.IsArray() {
				return "", fmt.Errorf("wildcard target requires an array index: %s", path)
			}
			if err == nil {
				if index < 0 {
					index += len(node.Array())
				}
				if index < 0 {
					return "", fmt.Errorf("wildcard target index is out of range: %s", path)
				}
				parts[i] = strconv.Itoa(index)
			}
		}
		node = node.Get(parts[i])
	}
	return strings.Join(parts, "."), nil
}

// Reject overlapping writes and writes into another source. This keeps batch
// results independent of traversal order, without inventing merge semantics.
func validateWildcardDestinations(matches []ParamOperation) error {
	sources := make(map[string]bool, len(matches))
	sortedSources := make([]string, 0, len(matches))
	targets := make([]string, 0, len(matches))
	for _, match := range matches {
		sources[match.From] = true
		sortedSources = append(sortedSources, match.From)
		targets = append(targets, match.To)
	}
	sort.Strings(sortedSources)
	sort.Strings(targets)
	for i, target := range targets {
		if i > 0 && target == targets[i-1] {
			return fmt.Errorf("wildcard targets overlap: %s", target)
		}
		j := sort.SearchStrings(targets, target+".")
		if j < len(targets) && strings.HasPrefix(targets[j], target+".") {
			return fmt.Errorf("wildcard targets overlap: %s", target)
		}
	}
	for _, match := range matches {
		parts := relayparam.JSONPathSegments(match.To)
		for i := range parts {
			prefix := strings.Join(parts[:i+1], ".")
			if sources[prefix] && prefix != match.From {
				return fmt.Errorf("wildcard target overlaps another source: %s", match.To)
			}
		}
		j := sort.SearchStrings(sortedSources, match.To+".")
		for j < len(sortedSources) && strings.HasPrefix(sortedSources[j], match.To+".") {
			if sortedSources[j] != match.From {
				return fmt.Errorf("wildcard target overlaps another source: %s", match.To)
			}
			j++
		}
	}
	return nil
}
