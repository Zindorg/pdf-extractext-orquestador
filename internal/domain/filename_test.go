package domain

import (
	"strings"
	"testing"
)

func TestCleanFilename(t *testing.T) {
	cases := map[string]string{
		"contrato.pdf":              "contrato.pdf",
		`C:\Users\ana\contrato.pdf`: "contrato.pdf",
		"../../etc/passwd":          "passwd",
		"":                          "",
		"/":                         "",
		strings.Repeat("ñ", 300):    strings.Repeat("ñ", 255),
	}
	for in, want := range cases {
		if got := CleanFilename(in); got != want {
			t.Errorf("CleanFilename(%q) = %q, quería %q", in, got, want)
		}
	}
}
