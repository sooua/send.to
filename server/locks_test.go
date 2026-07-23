package server

import (
	"sync"
	"testing"
)

// The per-upload lock map used to keep one mutex per token/filename forever.
func TestLockMapIsReleasedWhenIdle(t *testing.T) {
	srvr, err := New()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	for i := 0; i < 100; i++ {
		srvr.lock("token", "file.txt")
		srvr.unlock("token", "file.txt")
	}

	srvr.locksMu.Lock()
	defer srvr.locksMu.Unlock()
	if len(srvr.locks) != 0 {
		t.Errorf("lock map holds %d entries after all locks were released, want 0", len(srvr.locks))
	}
}

// Concurrent holders must serialise, and the entry must survive until the last
// one is done — the refcount exists so eviction cannot unlock a stale mutex.
func TestLockMapIsMutuallyExclusive(t *testing.T) {
	srvr, err := New()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	const goroutines = 32

	var wg sync.WaitGroup
	counter := 0

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			srvr.lock("token", "file.txt")
			counter++
			srvr.unlock("token", "file.txt")
		}()
	}

	wg.Wait()

	if counter != goroutines {
		t.Errorf("counter = %d, want %d — updates were not serialised", counter, goroutines)
	}

	srvr.locksMu.Lock()
	defer srvr.locksMu.Unlock()
	if len(srvr.locks) != 0 {
		t.Errorf("lock map holds %d entries, want 0", len(srvr.locks))
	}
}
