package sunapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

func RegisterFrontend(engine *gin.Engine, content fs.FS) {
	if engine == nil || content == nil {
		return
	}

	serveIndex := func(c *gin.Context) {
		serveFrontendFile(c, content, "index.html")
	}
	engine.GET("/index.html", serveIndex)

	for _, route := range []string{"/home", "/dashboard", "/channels", "/groups", "/keys", "/usage-logs", "/settings", "/playground", "/docs"} {
		engine.GET(route, serveIndex)
		engine.GET(route+"/*path", serveIndex)
	}

	engine.GET("/static/*filepath", func(c *gin.Context) {
		serveFrontendFile(c, content, cleanFrontendPath("static", c.Param("filepath")))
	})
	engine.GET("/assets/*filepath", func(c *gin.Context) {
		serveFrontendFile(c, content, cleanFrontendPath("assets", c.Param("filepath")))
	})
	for _, name := range []string{"sun-logo.svg", "favicon.ico", "manifest.json", "robots.txt"} {
		fileName := name
		engine.GET("/"+fileName, func(c *gin.Context) {
			serveFrontendFile(c, content, fileName)
		})
	}
}

func serveFrontendFile(c *gin.Context, content fs.FS, name string) {
	name = cleanFrontendPath("", name)
	if name == "" || name == "." {
		name = "index.html"
	}
	if _, err := fs.Stat(content, name); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	http.ServeFileFS(c.Writer, c.Request, content, name)
}

func cleanFrontendPath(prefix, value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "/"))
	if prefix != "" {
		value = path.Join(prefix, value)
	}
	cleaned := path.Clean("/" + value)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		return ""
	}
	return cleaned
}
