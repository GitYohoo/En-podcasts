//go:build !windows

package main

import "os/exec"

func newBackgroundCommand(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}
