package billingexpr

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRequestSnapshotSurvivesStorageAndUsageBranchChanges(t *testing.T) {
	expression := `(u("seconds") < 10 ? tier("short", u("seconds")) : tier("long", u("seconds") * (param("quality") == "high" ? 2 : 1))) * (header("X-Plan") == "premium" ? 3 : 1) * (hour("UTC") == 8 ? 2 : 1)`
	at := time.Date(2000, 1, 2, 8, 30, 0, 0, time.UTC)
	input := RequestInput{Body: []byte(`{"quality":"high","prompt":"private prompt","image":"private image"}`), Headers: map[string]string{"X-Plan": "premium", "Authorization": "private token"}, At: &at}
	frozen, err := SnapshotRequestInput(expression, input, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, frozen.Body)
	assert.Equal(t, map[string]any{"quality": "high"}, frozen.Params)
	assert.Equal(t, map[string]string{"x-plan": "premium"}, frozen.Headers)
	initial := *frozen
	initial.Usage = map[string]any{"seconds": 5.0}
	cost, trace, err := RunExprWithRequest(expression, TokenParams{}, initial)
	require.NoError(t, err)
	assert.Equal(t, 30.0, cost)
	assert.Equal(t, "short", trace.MatchedTier)
	snapshot := BillingSnapshot{ExprString: expression, ExprHash: ExprHashString(expression), TaskUsageBilling: true, QuotaPerUnit: 100, GroupRatio: 0.5, RequestInput: frozen, EstimatedTier: "short"}
	encoded, err := common.Marshal(snapshot)
	require.NoError(t, err)
	for _, secret := range []string{"private prompt", "private image", "private token", "Authorization"} {
		assert.NotContains(t, string(encoded), secret)
	}
	var restored BillingSnapshot
	require.NoError(t, common.Unmarshal(encoded, &restored))
	at = at.Add(12 * time.Hour)
	input.Headers["X-Plan"] = "basic"
	result, err := ComputeTieredQuotaWithRequest(&restored, TokenParams{}, RequestInput{Body: []byte(`{"quality":"low"}`), Headers: input.Headers, At: &at, Usage: map[string]any{"seconds": 20.0}})
	require.NoError(t, err)
	assert.Equal(t, 24000.0, result.ActualQuotaBeforeGroup)
	assert.Equal(t, 12000, result.ActualQuotaAfterGroup)
	assert.Equal(t, "long", result.MatchedTier)
	require.Len(t, result.RequestRules, 3)
	for _, rule := range result.RequestRules {
		assert.True(t, rule.Matched, rule.Cond)
	}
}

func TestTaskRequestSnapshotDynamicProbesAndNestedValues(t *testing.T) {
	expression := `param(u("path")) * (header(u("header")) == "premium" ? 3 : 1)`
	body := []byte(`{"first":2,"second":4}`)
	headers := map[string]string{"x-first": "basic", "x-second": "premium"}
	frozen, err := SnapshotRequestInput(expression, RequestInput{Body: body, Headers: headers}, time.Time{})
	require.NoError(t, err)
	copy(body, []byte(`{"first":9,"second":9}`))
	headers["x-second"] = "basic"
	for _, tc := range []struct {
		path, header string
		cost         float64
	}{{"first", "x-first", 2}, {"second", "x-second", 12}} {
		input := *frozen
		input.Usage = map[string]any{"path": tc.path, "header": tc.header}
		cost, _, err := RunExprWithRequest(expression, TokenParams{}, input)
		require.NoError(t, err)
		assert.Equal(t, tc.cost, cost)
	}
	nested := map[string]any{"price": 2.0}
	frozen, err = SnapshotRequestInput(`param("options").price`, RequestInput{Params: map[string]any{"options": nested}}, time.Time{})
	require.NoError(t, err)
	nested["price"] = 9.0
	cost, _, err := RunExprWithRequest(`param("options").price`, TokenParams{}, *frozen)
	require.NoError(t, err)
	assert.Equal(t, 2.0, cost)
}
