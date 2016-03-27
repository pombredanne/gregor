package main

import (
	"net"
	"os"
)

type dummy struct{}

func (d dummy) Serve(l net.Listener) error {
	return nil
}

func main() {
	opts, err := ParseOptions(os.Args)
	if err != nil {
		errorf("%s\n", err)
		os.Exit(2)
	}
	err = newMainServer(opts, dummy{}).listenAndServe()
	if err != nil {
		errorf("%s\n", err)
		os.Exit(2)
	}
	return
}
