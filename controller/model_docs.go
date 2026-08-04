package controller

import (
	"bytes"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/modeldoc"

	"github.com/gin-gonic/gin"
)

func GetModelDocsCatalog(c *gin.Context) {
	documents, err := modeldoc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	enabledModels, err := model.GetDocumentEnabledModelNames()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	enabledSet := make(map[string]struct{}, len(enabledModels))
	for _, modelName := range enabledModels {
		enabledSet[modelName] = struct{}{}
	}
	filtered := make([]modeldoc.Document, 0, len(documents))
	for _, document := range documents {
		if _, enabled := enabledSet[document.Model]; enabled {
			filtered = append(filtered, document)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    filtered,
	})
}

func GetModelDoc(c *gin.Context) {
	meta, exists, err := modeldoc.Find(c.Param("slug"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "model document not found",
		})
		return
	}
	enabled, err := model.IsModelDocumentEnabled(meta.Model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if !enabled {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "model document not found",
		})
		return
	}
	document, _, err := modeldoc.Read(meta.Slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	stylesheet := modeldoc.Stylesheet()
	document = bytes.Replace(
		document,
		[]byte("</head>"),
		append(append([]byte("<style>"), stylesheet...), []byte("</style></head>")...),
		1,
	)
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data: https:; base-uri 'none'; frame-ancestors 'self'")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/html; charset=utf-8", document)
}
