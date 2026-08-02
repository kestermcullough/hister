//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package ytdlp

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		// Setpgid makes the yt-dlp PID the process-group ID. A negative PID
		// sends SIGKILL to yt-dlp and every descendant still in that group.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
