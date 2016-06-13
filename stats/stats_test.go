// Copyright 2016 Keybase Inc. All rights reserved.
// Use of this source code is governed by a BSD
// license that can be found in the LICENSE file.

package stats

import (
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	"testing"
)

func TestStatPrefix(t *testing.T) {
	backend, _ := NewBackend(MOCK, nil)
	reg := NewSimpleRegistry(backend, rpc.SimpleLogOutput{}).(SimpleRegistry)
	reg = reg.SetPrefix("gregor").(SimpleRegistry)

	if reg.prefix != "gregor - " {
		t.Fatalf("reg prefix incorrect: %s != %s", reg.prefix, "gregor - ")
	}

	subreg := reg.SetPrefix("server").(SimpleRegistry)
	if subreg.prefix != "gregor - server - " {
		t.Fatalf("sub reg prefix incorrect: %s != %s", subreg.prefix, "gregor - server - ")
	}

	fname := subreg.makeFname("new conn")
	if fname != "gregor - server - new conn" {
		t.Fatalf("fname incorrect: %s != %s", fname, "gregor - server - new conn")
	}

}
