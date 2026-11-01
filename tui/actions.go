package tui

import (
	"bytes"
	"fmt"
	"os/exec"
)

// runCommand runs an external command, feeding stdin if needed, and returns
// an error with the command output included. The caller surfaces the result
// via Recent Events.
func runCommand(stdinData string, name string, arg ...string) error {
	var cmd *exec.Cmd
	switch name {
	case "pbcopy":
		cmd = exec.Command("pbcopy")
	case "open":
		cmd = exec.Command("open", arg...)
	default:
		cmd = exec.Command(name, arg...)
	}
	if stdinData != "" {
		var buf bytes.Buffer
		buf.WriteString(stdinData)
		cmd.Stdin = bytes.NewReader(buf.Bytes())
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w — %s", name, err, out.String())
	}
	return nil
}

// copyConfigPath copies the given path to the system clipboard via pbcopy.
// Returns error if pbcopy fails.
func copyConfigPath(path string) error {
	return runCommand(path, "pbcopy")
}

// revealConfigFolder runs `open -R <path>` to reveal the file in Finder.
// Returns error if open fails.
func revealConfigFolder(path string) error {
	return runCommand("", "open", "-R", path)
}