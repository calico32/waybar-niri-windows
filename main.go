package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
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
