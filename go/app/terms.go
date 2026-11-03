package app

import (
	"fmt"
	"os"
	"strings"
)

// LoadTerms reads a search-terms file: one Marketplace query per line. Blank
// lines and lines starting with # are ignored, whitespace is trimmed, and
// duplicate terms are dropped while preserving the file order.
func LoadTerms(path string) ([]string, error) {
	body, err := os.ReadFile(path) //nolint:gosec // path is user-configured input.
	if err != nil {
		return nil, fmt.Errorf("read search terms %s: %w", path, err)
	}

	terms := ParseTerms(string(body))
	if len(terms) == 0 {
		return nil, fmt.Errorf("no search terms defined in %s", path)
	}
	return terms, nil
}

// ParseTerms extracts the usable terms from a search-terms document.
func ParseTerms(document string) []string {
	seen := make(map[string]struct{})
	terms := make([]string, 0)

	for line := range strings.SplitSeq(document, "\n") {
		term := strings.TrimSpace(line)
		if !isTerm(term) {
			continue
		}
		if _, exists := seen[term]; exists {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
	}
	return terms
}

// isTerm reports whether a trimmed line is a search term rather than noise.
func isTerm(term string) bool {
	return term != "" && !strings.HasPrefix(term, "#")
}
