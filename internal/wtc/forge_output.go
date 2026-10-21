package wtc

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
)

// forgeOutput runs a forge CLI and returns its stdout, read back from a
// temporary regular file instead of a pipe. Node-based CLIs such as bb write
// --json in one call and exit; on a pipe only the first 64KiB arrives, on a
// file the whole body does. Stderr stays out of the body: when the caller has
// not set one, it is captured into the returned *exec.ExitError as
// cmd.Output would.
func forgeOutput(cmd *exec.Cmd) ([]byte, error) {
	file, err := os.CreateTemp("", "wtc-forge-*.out")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	cmd.Stdout = file
	var stderr bytes.Buffer
	captured := cmd.Stderr == nil
	if captured {
		cmd.Stderr = &stderr
	}
	runErr := cmd.Run()
	var exitErr *exec.ExitError
	if captured && errors.As(runErr, &exitErr) {
		exitErr.Stderr = stderr.Bytes()
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	out, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return out, runErr
}
