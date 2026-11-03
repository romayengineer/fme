package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseTerms(t *testing.T) {
	document := "# marketplace queries\n\n  macbook m1  \niphone 16\n# comment\nmacbook m1\n\n  \nmac mini\n"

	got := ParseTerms(document)
	want := []string{"macbook m1", "iphone 16", "mac mini"}

	if !slices.Equal(got, want) {
		t.Errorf("ParseTerms = %v, want %v", got, want)
	}
}

func TestParseTermsEmpty(t *testing.T) {
	if got := ParseTerms("\n# only comments\n   \n"); len(got) != 0 {
		t.Errorf("ParseTerms = %v, want empty", got)
	}
}

func TestLoadTerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terms.txt")
	if err := os.WriteFile(path, []byte("macbook m1\nmac mini\n"), 0o600); err != nil {
		t.Fatalf("write terms: %v", err)
	}

	terms, err := LoadTerms(path)
	if err != nil {
		t.Fatalf("LoadTerms: %v", err)
	}
	if !slices.Equal(terms, []string{"macbook m1", "mac mini"}) {
		t.Errorf("LoadTerms = %v", terms)
	}
}

func TestLoadTermsErrors(t *testing.T) {
	dir := t.TempDir()

	if _, err := LoadTerms(filepath.Join(dir, "missing.txt")); err == nil {
		t.Error("LoadTerms returned no error for a missing file")
	}

	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte("# nothing here\n"), 0o600); err != nil {
		t.Fatalf("write terms: %v", err)
	}
	if _, err := LoadTerms(empty); err == nil {
		t.Error("LoadTerms returned no error for an empty terms file")
	}
}
