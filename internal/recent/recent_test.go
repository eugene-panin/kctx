package recent

import (
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentTouch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	var wg sync.WaitGroup
	errCh := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := Touch(fmt.Sprintf("client/c%d", i%8)); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 || len(list) > maxRecent {
		t.Fatalf("recent len=%d list=%v", len(list), list)
	}
}

func TestToggleFavorite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	on, err := ToggleFavorite("acme/prod")
	if err != nil || !on {
		t.Fatalf("on=%v err=%v", on, err)
	}
	on, err = ToggleFavorite("acme/prod")
	if err != nil || on {
		t.Fatalf("off: on=%v err=%v", on, err)
	}
}
