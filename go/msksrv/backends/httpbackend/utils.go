package httpbackend

import (
	"regexp"
)

var wildcardExtractor = regexp.MustCompile(`(?:^|/)\{([^{}]+)\}`)

func extractWildcards(s string) []string {
	var results []string
	for _, match := range wildcardExtractor.FindAllStringSubmatch(s, -1) {
		results = append(results, match[1])
	}
	return results
}
