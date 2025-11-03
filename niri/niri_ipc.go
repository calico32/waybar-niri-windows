package niri

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"reflect"
)

func Init() (*State, net.Conn, error) {
	socket := os.Getenv("NIRI_SOCKET")
	if socket == "" {
		return nil, nil, fmt.Errorf("NIRI_SOCKET not set")
	}
	eventSocket, err := net.Dial("unix", socket)
	if err != nil {
		return nil, nil, fmt.Errorf("error connecting to NIRI_SOCKET: %w", err)
	}

	// Can't send actions if we're listening to the EventStream, so we need a
	// separate socket for actions.
	niriSocket, err := net.Dial("unix", socket)
	if err != nil {
		return nil, nil, fmt.Errorf("error connecting to NIRI_SOCKET: %w", err)
	}

	niriState := NewNiriState()

	go listen(eventSocket, niriState)

	// log messages on the action socket
	go func() {
		b := bufio.NewReader(niriSocket)
		for {
			line, err := b.ReadString('\n')
			if err != nil {
				if errors.Is(err, io.EOF) {
					log.Println("wbcffi: niri connection closed")
				} else {
					log.Printf("wbcffi: error reading from niri socket: %s", err)
				}
				return
			}

			line = line[:len(line)-1]
			if line == "" {
				continue
			}

			log.Printf("wbcffi: niri   -> %s", line)
		}
	}()

	return niriState, niriSocket, nil
}

func listen(socket net.Conn, state *State) {
	defer socket.Close()
	socket.Write([]byte("\"EventStream\"\n"))
	b := bufio.NewReader(socket)
	for {
		line, err := b.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Println("wbcffi: niri connection closed")
			} else {
				log.Printf("wbcffi: error reading from niri socket: %s", err)
			}
			return
		}

		line = line[:len(line)-1]
		if line == "" {
			continue
		}

		niriEvent := new(NiriEvent)
		err = json.Unmarshal([]byte(line), niriEvent)
		if err != nil {
			log.Printf("wbcffi: error unmarshaling niri event: %s", err)
			continue
		}
		if niriEvent.Ok != nil {
			// response to EventStream request, ignore
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
			state.Update(event)
		} else {
			log.Printf("wbcffi: received event with no fields set (unknown event type?)")
		}
	}
}
