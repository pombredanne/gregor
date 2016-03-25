package main

import (
	"net"
	"os"
)

type dummy struct{}

func (d dummy) MainLoop(l net.Listener) error {
	return nil
}

func main() {
	opts, err := ParseOptions(os.Args)
	if err != nil {
		errorf("%s\n", err)
		os.Exit(2)
	}
	err = newMainServer(opts, dummy{}).run()
	if err != nil {
		errorf("%s\n", err)
		os.Exit(2)
	}
	return
}
