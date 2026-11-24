// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package dl

import "os/exec"

func hideWindow(*exec.Cmd) {}
