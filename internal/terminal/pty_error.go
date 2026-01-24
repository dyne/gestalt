package terminal

import "fmt"

const conPTYUnavailableHint = "windows PTY unavailable; ConPTY support is required (Windows 10+)"

func wrapPtyStartError(err error) error {
	if err == nil {
		return nil
	}
	if isConPTYUnavailable(err) {
		return fmt.Errorf("%s: %w", conPTYUnavailableHint, err)
	}
	return err
}
