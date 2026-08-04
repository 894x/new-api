package modeldoc

import _ "embed"

//go:embed document.css
var documentStylesheet []byte

func Stylesheet() []byte {
	return append([]byte(nil), documentStylesheet...)
}
