//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package ytdlp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagedCommandContextKillsDescendantsHoldingOutputPipe(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-survived")
	script := filepath.Join(dir, "parent-with-child")
	contents := "#!/bin/sh\n(sleep 1; touch " + shellQuote(marker) + ") &\nwait\n"
	if err := os.WriteFile(script, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := managedCommandContext(ctx, script).Output()
	if err == nil {
		t.Fatal("managed command unexpectedly succeeded")
	}
	if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
		t.Fatalf("managed command returned after %s; descendant likely retained stdout", elapsed)
	}

	// Give an escaped descendant enough time to write its marker. Killing the
	// full process group prevents it from surviving the parent's deadline.
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived command timeout; marker stat error: %v", err)
	}
}

func shellQuote(value string) string {
	quoted := "'"
	for _, r := range value {
		if r == '\'' {
			quoted += "'\\''"
		} else {
			quoted += string(r)
		}
	}
	return quoted + "'"
}
