package state

import (
	"sync"
	"wnw/module"
	"wnw/niri"
)

type mutex interface {
	Lock()
	Unlock()
	RLock()
	RUnlock()
}

type noopMutex struct{}

func (noopMutex) Lock()    {}
func (noopMutex) Unlock()  {}
func (noopMutex) RLock()   {}
func (noopMutex) RUnlock() {}

type State struct {
	mu     mutex
	locked bool

	instances  map[uintptr]*module.Instance
	niriState  *niri.State
	niriSocket niri.Socket
}

// New creates a State with its mutex set to a new sync.RWMutex and the instances map initialized to an empty map.
func New() State {
	return State{
		mu:        new(sync.RWMutex),
		instances: make(map[uintptr]*module.Instance),
	}
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

func (s *State) Locked(f func(*State)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// disable locking for the duration of the function
	mu := s.mu
	s.mu = noopMutex{}
	defer func() {
		s.mu = mu
	}()
	f(s)
}