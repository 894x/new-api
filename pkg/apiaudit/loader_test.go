package apiaudit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCaseFixture(t *testing.T, root, suite, folder, body string) {
	t.Helper()
	dir := filepath.Join(root, suite, folder)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "case.json"), []byte(body), 0o644))
}

func TestLoadSuiteSortsAndValidatesFolderScopedCases(t *testing.T) {
	root := t.TempDir()
	writeCaseFixture(t, root, "openai-chat", "T002-models", `{
  "id":"T002","name":"模型列表","dimension":"identity",
  "protocol":"openai-chat","kind":"models_contains","default":false,
  "severity":"normal","request":{"method":"GET","path":"/v1/models"}
}`)
	writeCaseFixture(t, root, "openai-chat", "T001-sync", `{
  "id":"T001","name":"同步响应","dimension":"boundary",
  "protocol":"openai-chat","kind":"chat_sync","default":true,
  "severity":"critical","request":{"method":"POST","path":"/v1/chat/completions","body":{"messages":[]}}
}`)

	cases, err := LoadSuite(root, "openai-chat")

	require.NoError(t, err)
	require.Len(t, cases, 2)
	assert.Equal(t, "T001", cases[0].ID)
	assert.Equal(t, "T002", cases[1].ID)
	assert.Equal(t, filepath.Join(root, "openai-chat", "T001-sync"), cases[0].Dir)
	assert.Equal(t, "critical", cases[0].Severity)
}

func TestLoadSuiteRejectsDuplicateIDs(t *testing.T) {
	root := t.TempDir()
	for _, folder := range []string{"a", "b"} {
		writeCaseFixture(t, root, "openai-chat", folder, `{
  "id":"T001","name":"重复","dimension":"boundary",
  "protocol":"openai-chat","kind":"chat_sync","default":true,
  "request":{"method":"POST","path":"/v1/chat/completions"}
}`)
	}

	_, err := LoadSuite(root, "openai-chat")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate case id T001")
}

func TestLoadSuiteRejectsProtocolMismatch(t *testing.T) {
	root := t.TempDir()
	writeCaseFixture(t, root, "openai-chat", "bad", `{
  "id":"T001","name":"错误协议","dimension":"boundary",
  "protocol":"seedance","kind":"chat_sync","default":true,
  "request":{"method":"POST","path":"/v1/chat/completions"}
}`)

	_, err := LoadSuite(root, "openai-chat")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "protocol")
}

func TestLoadSuiteRejectsUnsafeCaseID(t *testing.T) {
	root := t.TempDir()
	writeCaseFixture(t, root, "openai-chat", "bad", `{
  "id":"../escape","name":"危险目录","dimension":"boundary",
  "protocol":"openai-chat","kind":"chat_sync","default":true,
  "request":{"method":"POST","path":"/v1/chat/completions"}
}`)

	_, err := LoadSuite(root, "openai-chat")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsafe case id")
}

func TestLoadSuiteRejectsUnknownKind(t *testing.T) {
	root := t.TempDir()
	writeCaseFixture(t, root, "openai-chat", "bad", `{
  "id":"T001","name":"错误类型","dimension":"boundary",
  "protocol":"openai-chat","kind":"chat_syncc","default":true,
  "request":{"method":"POST","path":"/v1/chat/completions","body":{}}
}`)

	_, err := LoadSuite(root, "openai-chat")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported kind")
}

func TestLoadSuiteRejectsSeedanceCaseWithoutBody(t *testing.T) {
	root := t.TempDir()
	writeCaseFixture(t, root, "seedance", "bad", `{
  "id":"V001","name":"缺少请求体","dimension":"compatibility",
  "protocol":"seedance","kind":"seedance_task","default":true,
  "request":{"method":"POST","path":"/api/v3/contents/generations/tasks"}
}`)

	_, err := LoadSuite(root, "seedance")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "request body")
}

func TestLoadSuiteRejectsMistypedOption(t *testing.T) {
	root := t.TempDir()
	writeCaseFixture(t, root, "openai-chat", "bad", `{
  "id":"T001","name":"错误约束","dimension":"boundary",
  "protocol":"openai-chat","kind":"chat_sync","default":true,
  "request":{"method":"POST","path":"/v1/chat/completions","body":{}},
  "options":{"require_usage":"yes"}
}`)

	_, err := LoadSuite(root, "openai-chat")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "require_usage must be a boolean")
}

func TestSelectCasesUsesDefaultsOrExplicitIDs(t *testing.T) {
	cases := []CaseDefinition{
		{ID: "T001", Default: true},
		{ID: "T002", Default: false},
		{ID: "T003", Default: true},
	}

	defaults, err := SelectCases(cases, nil, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"T001", "T003"}, []string{defaults[0].ID, defaults[1].ID})

	explicit, err := SelectCases(cases, []string{"T002"}, false)
	require.NoError(t, err)
	require.Len(t, explicit, 1)
	assert.Equal(t, "T002", explicit[0].ID)

	all, err := SelectCases(cases, nil, true)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestSelectCasesRejectsUnknownID(t *testing.T) {
	_, err := SelectCases([]CaseDefinition{{ID: "T001", Default: true}}, []string{"T999"}, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown case T999")
}
