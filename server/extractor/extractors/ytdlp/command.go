package ytdlp

import (
	"context"
	"os/exec"
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
