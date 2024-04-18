package main

import "regexp"

// only keep a-z A-Z 0-9 and _ -, remove other characters
func safe_string(s string) string {
	return regexp.MustCompile(`[^a-zA-Z0-9_\-]`).ReplaceAllString(s, "")
}

// a-z A-Z 0-9 and _ - are safe
func is_safe_sting(s string) bool {
	return (s != "" && s == safe_string(s))
}
