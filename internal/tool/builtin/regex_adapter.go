package builtin

import "regexp"

// regexpAdapter is a tiny alias so the glob tool doesn't pull regexp
// through multiple import lines. Keeps the grep/glob files self-contained.
type regexpAdapter = regexp.Regexp

// compileRegex wraps regexp.Compile so callers don't need to import regexp.
func compileRegex(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(pattern)
}
