package store

import (
	"sync"
	"testing"
)

// Health checks and audit writes happen from many goroutines at once; none may fail.
func TestConcurrentWrites(t *testing.T) {
	s := openTest(t)
	e, err := s.EnsureDefaultEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); errs <- s.SetEnvironmentStatus(e.ID, StatusUp) }()
		go func() { defer wg.Done(); errs <- s.Audit("u", "POST", "/x", 200) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
