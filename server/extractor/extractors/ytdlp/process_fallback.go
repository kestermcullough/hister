//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package ytdlp

import "os/exec"

// configureProcessTree leaves exec.CommandContext's parent-process
// cancellation in place on unsupported platforms. WaitDelay still bounds how
// long inherited pipes can keep the caller blocked.
func configureProcessTree(_ *exec.Cmd) {}
