package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func SeedanceMediaContent(c *gin.Context) {
	name := c.Param("name")
	file, err := service.OpenSeedanceVideo(name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		c.Status(http.StatusNotFound)
		return
	}
	contentType := "video/mp4"
	if strings.HasSuffix(name, ".mov") {
		contentType = "video/quicktime"
	}
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "public, max-age=300")
	http.ServeContent(c.Writer, c.Request, name, info.ModTime(), file)
}
