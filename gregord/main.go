package main

import (
	"os"

	"github.com/keybase/gregor/rpc"
)

func main() {
	opts, err := ParseOptions(os.Args)
	if err != nil {
		errorf("%s\n", err)
		os.Exit(2)
	}
	err = newMainServer(opts, rpc.NewServer()).listenAndServe()
	if err != nil {
		errorf("%s\n", err)
		os.Exit(2)
	}
	return
}
