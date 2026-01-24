//go:build windows

package terminal

import "errors"

func isConPTYUnavailable(err error) bool {
	return errors.Is(err, errConPTYUnavailable)
}
