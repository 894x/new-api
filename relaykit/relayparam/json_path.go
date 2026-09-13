package relayparam

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

// ResolveJSONPaths expands segment wildcards into concrete object and array
// paths. When includeMissingLeaf is true, a missing final object field is
// retained so callers such as parameter overrides can create it.
func ResolveJSONPaths(data []byte, path string, includeMissingLeaf bool) ([]string, error) {
	if !HasJSONPathWildcard(path) {
		return []string{path}, nil
	}
	if !gjson.ValidBytes(data) {
		return nil, fmt.Errorf("invalid JSON document")
	}
	if strings.Contains(path, "#(") {
		return nil, fmt.Errorf("segment wildcards require a dotted path, without GJSON queries: %s", path)
	}

	segments := JSONPathSegments(path)
	return collectJSONPaths(gjson.ParseBytes(data), segments, nil, includeMissingLeaf), nil
}

// JSONPathSegments splits a dotted path without splitting escaped object keys.
// Segments retain their escapes so they can be used with both gjson and sjson.
func JSONPathSegments(path string) []string {
	var segments []string
	start := 0
	depth := 0
	quoted := false
	for i := 0; i < len(path); i++ {
		if path[i] == '\\' && i+1 < len(path) {
			i++
			continue
		}
		if path[i] == '"' {
			quoted = !quoted
		}
		if quoted {
			continue
		}
		if path[i] == '(' || path[i] == '[' || path[i] == '{' {
			depth++
		}
		if path[i] == ')' || path[i] == ']' || path[i] == '}' {
			depth--
		}
		if path[i] == '.' && depth == 0 {
			segments = append(segments, path[start:i])
			start = i + 1
		}
	}
	return append(segments, path[start:])
}

// HasJSONPathWildcard recognizes whole-segment wildcards, not GJSON queries
// or literal escaped stars in object keys.
func HasJSONPathWildcard(path string) bool {
	if !strings.Contains(path, "*") {
		return false
	}
	for _, segment := range JSONPathSegments(path) {
		if segment == "*" {
			return true
		}
	}
	return false
}

var jsonPathKeyEscaper = strings.NewReplacer(
	`\`, `\\`, `.`, `\.`, `*`, `\*`, `?`, `\?`, `#`, `\#`, `|`, `\|`, `!`, `\!`, `:`, `\:`,
	`"`, `\"`, `(`, `\(`, `)`, `\)`, `[`, `\[`, `]`, `\]`, `{`, `\{`, `}`, `\}`, `@`, `\@`,
)

func collectJSONPaths(node gjson.Result, segments []string, prefix []string, includeMissingLeaf bool) []string {
	if len(segments) == 0 {
		return []string{strings.Join(prefix, ".")}
	}

	segment := strings.TrimSpace(segments[0])
	if segment == "" {
		return nil
	}
	isLast := len(segments) == 1

	if segment == "*" {
		if node.IsArray() {
			array := node.Array()
			paths := make([]string, 0, len(array))
			for index, value := range array {
				paths = append(paths, collectJSONPaths(value, segments[1:], append(prefix, strconv.Itoa(index)), includeMissingLeaf)...)
			}
			return paths
		}
		if node.IsObject() {
			type entry struct {
				key   string
				value gjson.Result
			}
			entries := make([]entry, 0)
			node.ForEach(func(key, value gjson.Result) bool {
				entries = append(entries, entry{key: key.String(), value: value})
				return true
			})
			sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
			paths := make([]string, 0, len(entries))
			for _, item := range entries {
				paths = append(paths, collectJSONPaths(item.value, segments[1:], append(prefix, jsonPathKeyEscaper.Replace(item.key)), includeMissingLeaf)...)
			}
			return paths
		}
		return nil
	}

	if node.IsObject() {
		next := node.Get(segment)
		if isLast {
			if includeMissingLeaf || next.Exists() {
				return []string{strings.Join(append(prefix, segment), ".")}
			}
			return nil
		}
		if !next.Exists() {
			return nil
		}
		return collectJSONPaths(next, segments[1:], append(prefix, segment), includeMissingLeaf)
	}

	if node.IsArray() {
		index, err := strconv.Atoi(segment)
		array := node.Array()
		if index < 0 {
			index += len(array)
		}
		if err != nil || index < 0 || index >= len(array) {
			return nil
		}
		if isLast {
			return []string{strings.Join(append(prefix, strconv.Itoa(index)), ".")}
		}
		return collectJSONPaths(array[index], segments[1:], append(prefix, strconv.Itoa(index)), includeMissingLeaf)
	}
	return nil
}
