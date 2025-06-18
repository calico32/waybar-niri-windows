package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
)

func listen(socket net.Conn, events chan<- Event) {
	socket.Write([]byte("\"EventStream\"\n"))
	b := bufio.NewReader(socket)
	for {
		line, err := b.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		line = line[:len(line)-1]
		if line == "" {
			continue
		}

		niriEvent := new(NiriEvent)
		err = json.Unmarshal([]byte(line), niriEvent)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if niriEvent.Ok != nil {
			// response to the EventStream message
			continue
		}
		var event Event
		// assign value of first non-nil field of niriEvent to event
		for i := range reflect.TypeOf(niriEvent).Elem().NumField() {
			field := reflect.ValueOf(niriEvent).Elem().Field(i)
			if !field.IsNil() {
				event = field.Interface().(Event)
				break
			}
		}
		if event != nil {
			events <- event
		} else {
			fmt.Fprintln(os.Stderr, "Received event with no fields set (unknown event type?)")
		}
	}
}

func write(s string) {
	j := struct {
		Text string `json:"text"`
	}{
		Text: s,
	}

	b, err := json.Marshal(j)
	if err != nil {
		panic(err)
	}

	os.Stdout.Write(append(b, '\n'))
	os.Stdout.Sync()
}
