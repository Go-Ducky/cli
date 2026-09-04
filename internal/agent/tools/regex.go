package tools

import (
	"fmt"
	"regexp"
)

type regexMatcher struct{ re *regexp.Regexp }

func (r *regexMatcher) MatchString(s string) bool { return r.re.MatchString(s) }

func newRegex(pattern string) (*regexMatcher, error) {
	if pattern == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &regexMatcher{re: re}, nil
}
