package misirka

import (
	"regexp"
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

var wildcardExtractor = regexp.MustCompile(`(?:^|/)\{([^{}]+)\}`)

func extractWildcards(s string) []string {
	var results []string
	for _, match := range wildcardExtractor.FindAllStringSubmatch(s, -1) {
		results = append(results, match[1])
	}
	return results
}
