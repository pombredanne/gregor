package rpc

import (
	"fmt"
	"strings"
)

// IsSocketClosedError returns true if e looks like an error due
// to the socket being closed.
// net.errClosing isn't exported, so do this.. UGLY!
func IsSocketClosedError(e error) bool {
	return strings.HasSuffix(e.Error(), "use of closed network connection")
}

// hostErr is used to include which host had a particular error.
type hostErr struct {
	host string
	err  error
}

// pubErr is used to accumulate 1+ errors when publishing a
// message to multiple hosts.
type pubErr struct {
	errors []hostErr
}

// Error returns a list of all the errors that happened while
// publishing a message to multiple hosts.
func (p *pubErr) Error() string {
	s := make([]string, len(p.errors))
	for i, e := range p.errors {
		s[i] = fmt.Sprintf("host %q: error %s", e.host, e.err)
	}
	return fmt.Sprintf("(errors: %d) %s", len(p.errors), strings.Join(s, ", "))
}

// Add inserts an error for a host.
func (p *pubErr) Add(host string, e error) {
	p.errors = append(p.errors, hostErr{host: host, err: e})
}

// Empty returns true if no errors were added.
func (p *pubErr) Empty() bool {
	return len(p.errors) == 0
}
