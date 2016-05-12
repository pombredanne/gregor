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

type hostErr struct {
	host string
	err  error
}

type pubErr struct {
	errors []hostErr
}

func (p *pubErr) Error() string {
	s := make([]string, len(p.errors))
	for i, e := range p.errors {
		s[i] = fmt.Sprintf("host %q: error %s", e.host, e.err)
	}
	return fmt.Sprintf("(errors: %d) %s", len(p.errors), strings.Join(s, ", "))
}

func (p *pubErr) Add(host string, e error) {
	p.errors = append(p.errors, hostErr{host: host, err: e})
}

func (p *pubErr) Empty() bool {
	return len(p.errors) == 0
}
