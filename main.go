package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"rsc.io/getopt"
)

func main() {
	err := parseFlags(&getopt.CommandLine, os.Args[1:])
	if err == flag.ErrHelp {
		fmt.Fprintln(os.Stderr, "Usage: waybar-niri-windows [options]")
		getopt.CommandLine.SetOutput(os.Stderr)
		getopt.CommandLine.PrintDefaults()
		return
	} else if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	socket := os.Getenv("NIRI_SOCKET")
	if socket == "" {
		fmt.Fprintln(os.Stderr, "NIRI_SOCKET not set")
		os.Exit(1)
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close()

	state := NewNiriState()

	events := make(chan Event)
	go listen(conn, events)

	for event := range events {
		state.Update(event)
		state.Redraw()
	}
}
