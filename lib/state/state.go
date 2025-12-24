package state

import (
	"fmt"
	"sync"
	"wnw/log"
	"wnw/module"
	"wnw/niri"
)

type State struct {
	mu     *sync.RWMutex
	locked bool

	instances  map[uintptr]*module.Instance
	niriState  *niri.State
	niriSocket niri.Socket
}

func New() State {
	return State{
		mu:        new(sync.RWMutex),
		instances: make(map[uintptr]*module.Instance),
	}
}

func (s *State) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.niriState == nil {
		var err error
		log.Debugf("connecting to niri socket")
		niriState, niriSocket, err := niri.Init()
		if err != nil {
			return fmt.Errorf("connecting to niri socket: %s", err)
		}
		s.niriState = niriState
		s.niriSocket = niriSocket
	}

	return nil
}

func (s *State) AddInstance(i *module.Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instances[i.Id()] = i
}

func (s *State) RemoveInstance(id uintptr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.instances, id)
}

func (s *State) GetInstance(id uintptr) *module.Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.instances[id]
}

func (s *State) GetInstances() []*module.Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var instances []*module.Instance
	for _, i := range s.instances {
		instances = append(instances, i)
	}
	return instances
}

func (s *State) SetNiriState(niriState *niri.State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.niriState = niriState
}

func (s *State) GetNiriState() *niri.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.niriState
}

func (s *State) SetNiriSocket(niriSocket niri.Socket) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.niriSocket = niriSocket
}

func (s *State) GetNiriSocket() niri.Socket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.niriSocket
}
