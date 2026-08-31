package ytdlp

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

const processExitGracePeriod = time.Second

// managedCommandContext creates a command whose timeout applies to the entire
// process tree. yt-dlp distributions may launch a child process that inherits
// stdout; killing only the parent can otherwise leave Cmd.Wait blocked on the
// child's open pipe indefinitely.
func managedCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	configureProcessTree(cmd)
	// This is a final bound if platform-specific tree termination fails or a
	// descendant escapes the process tree before cancellation.
	cmd.WaitDelay = processExitGracePeriod
	return cmd
}

// setCommandTempDir confines temporary files created by yt-dlp and its
// PyInstaller bootloader to the invocation directory. The caller owns dir and
// removes it after Cmd returns, including after a timeout kills the process
// tree. Without this, interrupted one-file executables can abandon large _MEI
// trees in the container-wide /tmp tmpfs until Hister is restarted.
func setCommandTempDir(cmd *exec.Cmd, dir string) {
	env := make([]string, 0, len(os.Environ())+3)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "TMPDIR=") || strings.HasPrefix(value, "TMP=") ||
			strings.HasPrefix(value, "TEMP=") {
			continue
		}
		env = append(env, value)
	}
	cmd.Env = append(env, "TMPDIR="+dir, "TMP="+dir, "TEMP="+dir)
}
