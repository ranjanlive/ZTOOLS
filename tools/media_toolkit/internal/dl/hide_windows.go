// SPDX-License-Identifier: Apache-2.0

//go:build windows

package dl

import (
	"os/exec"
	"syscall"
)

// hideWindow stops console helpers (python, yt-dlp) from flashing a window
// when the tool runs under a GUI host.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
