package modeldoc

import (
	"bytes"
	_ "embed"
	"strings"
)

//go:embed document.css
var documentStylesheet []byte

func Stylesheet() []byte {
	return append([]byte(nil), documentStylesheet...)
}

func Render(document []byte) []byte {
	document = bytes.TrimSpace(document)
	if len(document) == 0 {
		return nil
	}
	lowerDocument := strings.ToLower(string(document))
	if !strings.Contains(lowerDocument, "<html") {
		document = append([]byte("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"UTF-8\"></head><body>"), document...)
		document = append(document, []byte("</body></html>")...)
		lowerDocument = strings.ToLower(string(document))
	}
	style := append(append([]byte("<style>"), documentStylesheet...), []byte("</style>")...)
	if headEnd := strings.Index(lowerDocument, "</head>"); headEnd >= 0 {
		result := make([]byte, 0, len(document)+len(style))
		result = append(result, document[:headEnd]...)
		result = append(result, style...)
		result = append(result, document[headEnd:]...)
		return result
	}
	if bodyStart := strings.Index(lowerDocument, "<body"); bodyStart >= 0 {
		result := make([]byte, 0, len(document)+len(style))
		result = append(result, document[:bodyStart]...)
		result = append(result, style...)
		result = append(result, document[bodyStart:]...)
		return result
	}
	return append(style, document...)
}
