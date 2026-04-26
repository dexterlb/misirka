package msksrv

import (
	"strings"
)

func assertPath(path string) {
	if strings.HasPrefix(path, "/") {
		panic("path has a leading slash: " + path)
	}
	if strings.HasSuffix(path, "/") {
		panic("path has a trailing slash: " + path)
	}
	if strings.Contains(path, "//") {
		panic("path has consecutive slashes: " + path)
	}
}
