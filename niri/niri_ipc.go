package niri

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"wnw/log"
)

type Socket struct {
	conn net.Conn
}

func (s *Socket) Request(j map[string]any) error {
	if s.conn == nil {
		return fmt.Errorf("socket is nil")
	}
	b, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}
	log.Debugf("niri <- %s", b)
	b = append(b, '\n')
	if _, err := s.conn.Write(b); err != nil {
		return fmt.Errorf("error writing to niri socket: %w", err)
	}
	return nil
}

func (s *Socket) logMessages() {
	go func() {
		b := bufio.NewReader(s.conn)
		for {
			line, err := b.ReadString('\n')
			if err != nil {
				if errors.Is(err, io.EOF) {
					log.Debugf("niri connection closed")
				} else {
					log.Debugf("error reading from niri socket: %s", err)
				}
				return
			}

			line = line[:len(line)-1]
			if line == "" {
				continue
			}

			log.Debugf("niri   -> %s", line)
		}
	}()
}

func Init() (state *State, socket Socket, err error) {
	socketAddr := os.Getenv("NIRI_SOCKET")
	if socketAddr == "" {
		err = fmt.Errorf("NIRI_SOCKET not set")
		return
	}

	eventSocket, err := net.Dial("unix", socketAddr)
	if err != nil {
		err = fmt.Errorf("error connecting to NIRI_SOCKET: %w", err)
		return
	}

	// Can't send actions if we're listening to the EventStream, so we need a
	// separate socket for actions.
	requestSocket, err := net.Dial("unix", socketAddr)
	if err != nil {
		eventSocket.Close() // close the other socket
		err = fmt.Errorf("error connecting to NIRI_SOCKET: %w", err)
		return
	}
	socket = Socket{requestSocket}
	socket.logMessages()
	state = NewNiriState()
	go listen(eventSocket, state)

	return
}

func listen(socket net.Conn, state *State) {
	defer socket.Close()
	if _, err := socket.Write([]byte("\"EventStream\"\n")); err != nil {
		log.Errorf("error writing to niri socket: %s", err)
		return
	}
	b := bufio.NewReader(socket)
	for {
		line, err := b.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Errorf("niri connection closed")
			} else {
				log.Errorf("error reading from niri socket: %s", err)
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
			log.Debugf("error unmarshaling niri event: %s", err)
			continue
		}
		if niriEvent.Ok != nil {
			// response to EventStream request, ignore
			continue
		}
		var event Event
		var ok bool
		// assign value of first non-nil field of niriEvent to event
		for i := range reflect.TypeOf(niriEvent).Elem().NumField() {
			field := reflect.ValueOf(niriEvent).Elem().Field(i)
			if !field.IsNil() {
				event, ok = field.Interface().(Event)
				if !ok {
					panic("fields on niri.NiriEvent must implement niri.Event")
				}
				break
			}
		}
		if event != nil {
			state.Update(event)
		} else {
			log.Warnf("received event with no fields set (unknown event type?)")
		}
	}
}
