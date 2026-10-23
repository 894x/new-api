package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskWecomBotWebhook(t *testing.T) {
	assert.Equal(t, "", maskWecomBotWebhook(""))
	assert.Equal(t, "****", maskWecomBotWebhook("abc"))
	assert.Equal(t, "****e359", maskWecomBotWebhook("81af1357-0f73-45e5-b7cf-465d453fe359"))
	assert.Equal(t,
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=****e359",
		maskWecomBotWebhook("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=81af1357-0f73-45e5-b7cf-465d453fe359"))
	assert.Equal(t, "****", maskWecomBotWebhook("https://evil.example/nokey"))
}
