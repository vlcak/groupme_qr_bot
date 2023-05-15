package main

import (
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"unicode"
)

func Normalize(s string) string {
	t := transform.Chain(norm.NFD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r) // Mn: nonspacing marks
	}), norm.NFC)
	val, _, _ := transform.String(t, s)
	return val
}

func NormalizeArray(a []string) []string {
	t := transform.Chain(norm.NFD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r) // Mn: nonspacing marks
	}), norm.NFC)
	for i, s := range a {
		a[i], _, _ = transform.String(t, s)
	}
	return a
}
