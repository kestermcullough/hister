package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExpandImportInputsExpandsDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"b.html",
		"a.json",
		"c.7z",
		"d.htm",
		"ignored.txt",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "subdir", "nested.html"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	inputs, err := expandImportInputs([]string{"first.json", dir, "last"})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"first.json",
		filepath.Join(dir, "a.json"),
		filepath.Join(dir, "b.html"),
		filepath.Join(dir, "c.7z"),
		filepath.Join(dir, "d.htm"),
		"last",
	}
	if !reflect.DeepEqual(inputs, want) {
		t.Fatalf("expandImportInputs() = %#v, want %#v", inputs, want)
	}
}

func TestIsSupportedImportInput(t *testing.T) {
	tests := map[string]bool{
		"export.json": true,
		"backup.7z":   true,
		"page.html":   true,
		"page.htm":    true,
		"page.HTML":   true,
		"notes.txt":   false,
		"README":      false,
	}

	for input, want := range tests {
		if got := isSupportedImportInput(input); got != want {
			t.Fatalf("isSupportedImportInput(%q) = %v, want %v", input, got, want)
		}
	}
}
