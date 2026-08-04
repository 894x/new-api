package modeldoc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogDocumentsAreIndependentlyReadable(t *testing.T) {
	documents, err := List()
	require.NoError(t, err)
	require.NotEmpty(t, documents)

	seen := make(map[string]struct{}, len(documents))
	for _, document := range documents {
		assert.NotEmpty(t, document.Model)
		assert.NotEmpty(t, document.InterfaceKey)
		assert.NotEmpty(t, document.InterfaceName)
		assert.NotEmpty(t, document.Vendor)
		assert.NotEmpty(t, document.Category)
		_, duplicate := seen[document.Slug]
		assert.False(t, duplicate, "duplicate slug %q", document.Slug)
		seen[document.Slug] = struct{}{}

		body, exists, readErr := Read(document.Slug)
		require.NoError(t, readErr)
		assert.True(t, exists)
		assert.Contains(t, string(body), "<!doctype html>")
		assert.Contains(t, string(body), document.Model)
	}
}

func TestReadRejectsUnknownDocument(t *testing.T) {
	body, exists, err := Read("../catalog")

	require.NoError(t, err)
	assert.False(t, exists)
	assert.Nil(t, body)
}

func TestRenderWrapsHTMLFragmentAndInjectsSharedStyles(t *testing.T) {
	rendered := string(Render([]byte("<main><h1>测试文档</h1></main>")))

	assert.Contains(t, rendered, "<!doctype html>")
	assert.Contains(t, rendered, "background: var(--background)")
	assert.Contains(t, rendered, "<main><h1>测试文档</h1></main>")
}
