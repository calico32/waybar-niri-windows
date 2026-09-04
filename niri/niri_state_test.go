package niri

import (
	"sync"
	"testing"
	"time"
)

// Update used to run the registered callbacks while still holding s.mu for
// reading. A callback that takes a lock of its own then deadlocks against a
// concurrent RemoveOnUpdate issued while that same lock is held, which is
// exactly what the waybar module does in Notify and Deinit.
func TestUpdateDoesNotHoldLockDuringCallbacks(t *testing.T) {
	state := NewNiriState()

	// stands in for module.Instance and its mutex
	var instance sync.RWMutex
	state.OnUpdate(1, func(*State) {
		instance.RLock()
		defer instance.RUnlock()
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 1000 {
			go state.Update(&ConfigLoaded{})

			instance.Lock()
			state.RemoveOnUpdate(1)
			instance.Unlock()

			state.OnUpdate(1, func(*State) {
				instance.RLock()
				defer instance.RUnlock()
			})
		}
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("deadlock: Update callbacks ran while the state lock was held")
	}
}
