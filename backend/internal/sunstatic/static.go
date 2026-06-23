package sunstatic

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

func Dist() (fs.FS, error) {
	return fs.Sub(embedded, "dist")
}
