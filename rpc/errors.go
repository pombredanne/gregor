package rpc

import "strings"

// IsSocketClosedError returns true if e looks like an error due
// to the socket being closed.
// net.errClosing isn't exported, so do this.. UGLY!
func IsSocketClosedError(e error) bool {
	return strings.HasSuffix(e.Error(), "use of closed network connection")
}
