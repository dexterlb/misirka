package msksrv

import (
	"fmt"
	"strings"
)

func (s *Server) assertPath(path string) {
	if strings.HasPrefix(path, "/") {
		panic("path has a leading slash: " + path)
	}
	if strings.HasSuffix(path, "/") {
		panic("path has a trailing slash: " + path)
	}
	if strings.Contains(path, "//") {
		panic("path has consecutive slashes: " + path)
	}
	if _, ok := s.topics[path]; ok {
		panic(fmt.Sprintf("path %s is already being used for a topic"))
	}
	if _, ok := s.calls[path]; ok {
		panic(fmt.Sprintf("path %s is already being used for a call"))
	}
}
