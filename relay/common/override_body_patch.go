package common

import (
	"bytes"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/relayparam"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// A wildcard operation evaluates all conditions against the same input. Cache
// shared ancestors (messages, content, etc.) so each match does not rescan the
// entire document. Results borrow from the immutable root string, never from
// an output buffer that subsequent operations can modify.
type overrideJSONSnapshot struct {
	root  gjson.Result
	nodes map[string]gjson.Result
}

func (s *overrideJSONSnapshot) get(path string) gjson.Result {
	if value, ok := s.nodes[path]; ok {
		return value
	}
	// Queries, projections and modifiers retain GJSON's complete-path rules.
	for i := 0; i < len(path); i++ {
		if path[i] == '\\' {
			i++
			continue
		}
		if strings.ContainsRune("#|@*!?:[]{}()\"", rune(path[i])) {
			value := s.root.Get(path)
			s.nodes[path] = value
			return value
		}
	}
	node := s.root
	parts := relayparam.JSONPathSegments(path)
	for i, part := range parts {
		prefix := strings.Join(parts[:i+1], ".")
		value, ok := s.nodes[prefix]
		if !ok {
			value = node.Get(part)
			s.nodes[prefix] = value
		}
		node = value
	}
	return node
}

// wrapJSONValues batches independent parent-to-child moves, e.g. image_url
// to image_url.url. Only the object/array wrappers are new; inline media stays
// in its original JSON representation and is copied to the output once.
// Conditions and wildcard conflict checks must already have run. Other moves
// retain the sequential engine's semantics.
func wrapJSONValues(data []byte, operations []ParamOperation) ([]byte, bool) {
	if len(operations) == 0 {
		return data, true
	}
	for _, op := range operations {
		if op.Mode != "move" || len(op.Conditions) != 0 || !strings.HasPrefix(op.To, op.From+".") {
			return data, false
		}
	}
	type wrapper struct {
		start, end     int
		prefix, suffix []byte
	}
	wrappers := make([]wrapper, 0, len(operations))
	// gjson.Get on a string returns views; GetBytes would copy each media
	// value. Keep this immutable snapshot alive until the output is complete.
	snapshot := string(data)
	size := len(data)
	for _, op := range operations {
		source := gjson.Get(snapshot, op.From)
		if !source.Exists() || source.Index <= 0 || source.Indexes != nil ||
			source.Index+len(source.Raw) > len(data) ||
			snapshot[source.Index:source.Index+len(source.Raw)] != source.Raw {
			return data, false
		}
		path := strings.TrimPrefix(op.To, op.From+".")
		// Ask sjson to construct the same wrapper as the sequential move, but
		// with a tiny placeholder instead of a potentially huge media value.
		template, err := sjson.SetRawBytes([]byte(`{}`), path, []byte(`null`))
		if err != nil {
			// Let the sequential engine retain its original error and audit behavior.
			return data, false
		}
		value := gjson.GetBytes(template, path)
		if value.Raw != "null" || value.Index <= 0 || value.Indexes != nil ||
			value.Index+4 > len(template) || !bytes.Equal(template[value.Index:value.Index+4], []byte("null")) {
			return data, false
		}
		wrappers = append(wrappers, wrapper{
			start: source.Index, end: source.Index + len(source.Raw),
			prefix: template[:value.Index], suffix: template[value.Index+4:],
		})
		size += len(template) - 4
	}
	sort.Slice(wrappers, func(i, j int) bool { return wrappers[i].start < wrappers[j].start })
	for i := 1; i < len(wrappers); i++ {
		if wrappers[i].start < wrappers[i-1].end {
			return data, false
		}
	}
	result := make([]byte, 0, size)
	position := 0
	for _, wrapper := range wrappers {
		result = append(result, data[position:wrapper.start]...)
		result = append(result, wrapper.prefix...)
		result = append(result, data[wrapper.start:wrapper.end]...)
		result = append(result, wrapper.suffix...)
		position = wrapper.end
	}
	result = append(result, data[position:]...)
	return result, true
}
