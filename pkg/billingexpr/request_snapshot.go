package billingexpr

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/expr-lang/expr/ast"
	"github.com/tidwall/gjson"
)

// SnapshotRequestInput retains only probes used by the expression, including
// probes in branches not reached with estimated usage. Dynamic paths preserve
// their source so measured usage can select a different key on completion.
// This belongs in private billing state, never in public task or usage logs.
func SnapshotRequestInput(expression string, input RequestInput, at time.Time) (*RequestInput, error) {
	program, err := CompileFromCache(expression)
	if err != nil {
		return nil, err
	}
	snapshot := &RequestInput{}
	headers := normalizeHeaders(input.Headers)
	retainBody, retainHeaders := false, false
	ast.Find(program.Node(), func(node ast.Node) bool {
		call, ok := node.(*ast.CallNode)
		if !ok || len(call.Arguments) != 1 {
			return false
		}
		callee, ok := call.Callee.(*ast.IdentifierNode)
		if !ok {
			return false
		}
		switch callee.Value {
		case "param":
			literal, ok := call.Arguments[0].(*ast.StringNode)
			if !ok {
				retainBody = true
				return false
			}
			if snapshot.Params == nil {
				snapshot.Params = make(map[string]any)
			}
			path := strings.TrimSpace(literal.Value)
			if value, exists := input.Params[path]; exists {
				snapshot.Params[path] = value
			} else if path != "" {
				snapshot.Params[path] = gjson.GetBytes(input.Body, path).Value()
			}
		case "header":
			literal, ok := call.Arguments[0].(*ast.StringNode)
			if !ok {
				retainHeaders = true
				return false
			}
			if snapshot.Headers == nil {
				snapshot.Headers = make(map[string]string)
			}
			name := strings.ToLower(strings.TrimSpace(literal.Value))
			snapshot.Headers[name] = headers[name]
		case "hour", "minute", "weekday", "month", "day":
			if input.At != nil {
				at = *input.At
			}
			if at.IsZero() {
				at = time.Now()
			}
			frozen := at
			snapshot.At = &frozen
		}
		return false
	})
	if retainBody {
		snapshot.Body = append([]byte(nil), input.Body...)
		for path, value := range input.Params {
			if snapshot.Params == nil {
				snapshot.Params = make(map[string]any)
			}
			snapshot.Params[path] = value
		}
	}
	if retainHeaders {
		snapshot.Headers = headers
	}
	// Detach nested JSON values as well as their map, and use the same numeric
	// representation before reservation and after database deserialization.
	if len(snapshot.Params) > 0 {
		data, err := common.Marshal(snapshot.Params)
		if err != nil {
			return nil, err
		}
		snapshot.Params = nil
		if err := common.Unmarshal(data, &snapshot.Params); err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}
