//go:build !unix

package process

import (
	"os"
	"os/exec"
)

func configureSysProcAttr(cmd *exec.Cmd) {}

func killProcessGroup(process *os.Process) error {
	if process == nil {
		return nil
	}
	return process.Kill()
}
