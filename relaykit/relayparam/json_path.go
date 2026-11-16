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
	resolver := newJSONPathResolver(data)
	return resolver.resolve(path, includeMissingLeaf)
}

type jsonPathResolution struct {
	path               string
	includeMissingLeaf bool
}

type jsonPathNodeKey struct {
	index int
	size  int
}

type jsonPathMemberKey struct {
	parent  jsonPathNodeKey
	segment string
}

type jsonPathChild struct {
	segment string
	value   gjson.Result
}

// A resolver belongs to one immutable JSON body. It shares wildcard expansion,
// container scans and concrete values across rules without decoding large text
// or Base64 strings into an application object tree.
type jsonPathResolver struct {
	data          []byte
	validated     bool
	validationErr error
	root          gjson.Result
	values        map[string]gjson.Result
	children      map[jsonPathNodeKey][]jsonPathChild
	members       map[jsonPathMemberKey]gjson.Result
	ambiguousKeys bool
	paths         map[jsonPathResolution][]string
}

func newJSONPathResolver(data []byte) jsonPathResolver {
	return jsonPathResolver{data: data}
}

func (r *jsonPathResolver) resolve(path string, includeMissingLeaf bool) ([]string, error) {
	if !HasJSONPathWildcard(path) {
		return []string{path}, nil
	}
	if !r.validated {
		r.validated = true
		if !gjson.ValidBytes(r.data) {
			r.validationErr = fmt.Errorf("invalid JSON document")
		} else {
			r.root = gjson.ParseBytes(r.data)
		}
	}
	if r.validationErr != nil {
		return nil, r.validationErr
	}
	if strings.Contains(path, "#(") {
		return nil, fmt.Errorf("segment wildcards require a dotted path, without GJSON queries: %s", path)
	}
	key := jsonPathResolution{path, includeMissingLeaf}
	if paths, ok := r.paths[key]; ok {
		return paths, nil
	}
	if r.values == nil {
		r.values = make(map[string]gjson.Result)
	}
	paths := r.collect(r.root, JSONPathSegments(path), nil, includeMissingLeaf)
	if r.paths == nil {
		r.paths = make(map[jsonPathResolution][]string)
	}
	r.paths[key] = paths
	return paths, nil
}

func (r *jsonPathResolver) get(path string) gjson.Result {
	if value, ok := r.values[path]; ok {
		return value
	}
	value := gjson.GetBytes(r.data, path)
	if r.values == nil {
		r.values = make(map[string]gjson.Result)
	}
	r.values[path] = value
	return value
}

func (r *jsonPathResolver) isMessagesVideoSource(path string) bool {
	blockPath, ok := strings.CutSuffix(path, ".source")
	return ok && (r.get(blockPath+".type").String() == "video" ||
		strings.HasPrefix(r.get(path+".media_type").String(), "video/"))
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

func (r *jsonPathResolver) collect(node gjson.Result, segments []string, prefix []string, includeMissingLeaf bool) []string {
	if len(segments) == 0 {
		return []string{strings.Join(prefix, ".")}
	}
	segment := strings.TrimSpace(segments[0])
	if segment == "" {
		return nil
	}
	isLast := len(segments) == 1
	parent := jsonPathNodeKey{node.Index, len(node.Raw)}
	if segment == "*" {
		children, cached := r.children[parent]
		if !cached {
			switch {
			case node.IsArray():
				for i, value := range node.Array() {
					children = append(children, jsonPathChild{strconv.Itoa(i), value})
				}
			case node.IsObject():
				node.ForEach(func(key, value gjson.Result) bool {
					children = append(children, jsonPathChild{key.String(), value})
					return true
				})
				sort.Slice(children, func(i, j int) bool { return children[i].segment < children[j].segment })
				for i := range children {
					if i > 0 && children[i].segment == children[i-1].segment && !r.ambiguousKeys {
						// Wildcard expansion visits duplicate keys, but concrete GJSON
						// lookups follow GJSON matching rules. Preserve both semantics.
						r.ambiguousKeys = true
						clear(r.values)
					}
				}
				for i := range children {
					children[i].segment = jsonPathKeyEscaper.Replace(children[i].segment)
				}
			default:
				return nil
			}
			if r.children == nil {
				r.children = make(map[jsonPathNodeKey][]jsonPathChild)
			}
			r.children[parent] = children
		}
		paths := make([]string, 0, len(children))
		for _, child := range children {
			childPrefix := append(prefix, child.segment)
			if !r.ambiguousKeys {
				r.values[strings.Join(childPrefix, ".")] = child.value
			}
			paths = append(paths, r.collect(child.value, segments[1:], childPrefix, includeMissingLeaf)...)
		}
		return paths
	}
	if node.IsObject() {
		childPrefix := append(prefix, segment)
		childPath := strings.Join(childPrefix, ".")
		key := jsonPathMemberKey{parent, segment}
		next, cached := r.members[key]
		if !cached {
			next = node.Get(segment)
			if r.members == nil {
				r.members = make(map[jsonPathMemberKey]gjson.Result)
			}
			r.members[key] = next
		}
		if !r.ambiguousKeys {
			r.values[childPath] = next
		}
		if isLast {
			if includeMissingLeaf || next.Exists() {
				return []string{childPath}
			}
			return nil
		}
		if !next.Exists() {
			return nil
		}
		return r.collect(next, segments[1:], childPrefix, includeMissingLeaf)
	}
	if node.IsArray() {
		index, err := strconv.Atoi(segment)
		children, cached := r.children[parent]
		if !cached {
			for i, value := range node.Array() {
				children = append(children, jsonPathChild{strconv.Itoa(i), value})
			}
			if r.children == nil {
				r.children = make(map[jsonPathNodeKey][]jsonPathChild)
			}
			r.children[parent] = children
		}
		if index < 0 {
			index += len(children)
		}
		if err != nil || index < 0 || index >= len(children) {
			return nil
		}
		child := children[index]
		childPrefix := append(prefix, child.segment)
		childPath := strings.Join(childPrefix, ".")
		if !r.ambiguousKeys {
			r.values[childPath] = child.value
		}
		if isLast {
			return []string{childPath}
		}
		return r.collect(child.value, segments[1:], childPrefix, includeMissingLeaf)
	}
	return nil
}
