package modeldoc

import (
	"embed"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type Document struct {
	Slug          string `json:"slug"`
	Model         string `json:"model"`
	InterfaceKey  string `json:"interface_key"`
	InterfaceName string `json:"interface_name"`
	Title         string `json:"title"`
	Vendor        string `json:"vendor"`
	Category      string `json:"category"`
	Summary       string `json:"summary"`
	UpdatedAt     string `json:"updated_at"`
}

type catalogFile struct {
	Documents []Document `json:"documents"`
}

//go:embed catalog.json documents/*.html
var content embed.FS

var (
	catalogOnce    sync.Once
	catalogDocs    []Document
	catalogByID    map[string]Document
	catalogByModel map[string][]Document
	catalogErr     error
)

func loadCatalog() {
	data, err := content.ReadFile("catalog.json")
	if err != nil {
		catalogErr = err
		return
	}

	var catalog catalogFile
	if err := common.Unmarshal(data, &catalog); err != nil {
		catalogErr = err
		return
	}

	catalogByID = make(map[string]Document, len(catalog.Documents))
	catalogByModel = make(map[string][]Document, len(catalog.Documents))
	for _, document := range catalog.Documents {
		document.Slug = strings.TrimSpace(document.Slug)
		document.InterfaceKey = strings.TrimSpace(document.InterfaceKey)
		document.InterfaceName = strings.TrimSpace(document.InterfaceName)
		if document.InterfaceKey == "" {
			document.InterfaceKey = "default"
		}
		if document.InterfaceName == "" {
			document.InterfaceName = "默认接口"
		}
		if document.Slug == "" || path.Base(document.Slug) != document.Slug || strings.Contains(document.Slug, "..") {
			catalogErr = fmt.Errorf("invalid model document slug %q", document.Slug)
			return
		}
		if _, exists := catalogByID[document.Slug]; exists {
			catalogErr = fmt.Errorf("duplicate model document slug %q", document.Slug)
			return
		}
		if _, err := content.ReadFile("documents/" + document.Slug + ".html"); err != nil {
			catalogErr = fmt.Errorf("read model document %q: %w", document.Slug, err)
			return
		}
		catalogDocs = append(catalogDocs, document)
		catalogByID[document.Slug] = document
		catalogByModel[document.Model] = append(catalogByModel[document.Model], document)
	}
}

func Find(slug string) (Document, bool, error) {
	catalogOnce.Do(loadCatalog)
	if catalogErr != nil {
		return Document{}, false, catalogErr
	}
	document, exists := catalogByID[slug]
	return document, exists, nil
}

func HasModel(modelName string) bool {
	catalogOnce.Do(loadCatalog)
	if catalogErr != nil {
		return false
	}
	documents, exists := catalogByModel[modelName]
	return exists && len(documents) > 0
}

func FindByModel(modelName string) (Document, bool, error) {
	catalogOnce.Do(loadCatalog)
	if catalogErr != nil {
		return Document{}, false, catalogErr
	}
	documents, exists := catalogByModel[modelName]
	if !exists || len(documents) == 0 {
		return Document{}, false, nil
	}
	return documents[0], true, nil
}

func ListByModel(modelName string) ([]Document, error) {
	catalogOnce.Do(loadCatalog)
	if catalogErr != nil {
		return nil, catalogErr
	}
	return append([]Document(nil), catalogByModel[modelName]...), nil
}

func List() ([]Document, error) {
	catalogOnce.Do(loadCatalog)
	if catalogErr != nil {
		return nil, catalogErr
	}
	return append([]Document(nil), catalogDocs...), nil
}

func Read(slug string) ([]byte, bool, error) {
	catalogOnce.Do(loadCatalog)
	if catalogErr != nil {
		return nil, false, catalogErr
	}
	if _, exists := catalogByID[slug]; !exists {
		return nil, false, nil
	}
	document, err := content.ReadFile("documents/" + slug + ".html")
	if err != nil {
		return nil, false, err
	}
	return document, true, nil
}
