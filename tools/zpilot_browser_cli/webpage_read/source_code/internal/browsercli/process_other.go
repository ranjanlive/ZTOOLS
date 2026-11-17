//go:build !windows

package browsercli

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
