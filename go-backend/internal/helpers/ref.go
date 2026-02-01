package helpers

import (
	"regexp"
	"strings"
)

var (
	nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9\s-]`)
	whitespace      = regexp.MustCompile(`\s+`)
	multipleDashes  = regexp.MustCompile(`-+`)
)

func GenerateRef(name string) string {
	s := nonAlphanumeric.ReplaceAllString(name, "")
	s = whitespace.ReplaceAllString(s, "-")
	s = multipleDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return strings.ToLower(s)
}
