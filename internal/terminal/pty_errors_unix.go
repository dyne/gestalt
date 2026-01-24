//go:build !windows

package terminal

func isConPTYUnavailable(err error) bool {
	return false
}
