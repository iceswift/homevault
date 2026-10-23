package lock

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestExclusiveAndRelease(t *testing.T) {
	state := t.TempDir()
	first, err := Acquire(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(state); !errors.Is(err, ErrLocked) {
		t.Fatalf("second acquisition = %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatal("release must be idempotent:", err)
	}
	second, err := Acquire(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestNeverRemoveStaleOrForeignLock(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "operation.lock")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(state); !errors.Is(err, ErrLocked) {
		t.Fatalf("unowned lock was not respected: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("existing lock removed:", err)
	}
	foreign := &Lock{path: path, token: "not-owner"}
	if err := foreign.Release(); err == nil {
		t.Fatal("foreign lock released")
	}
}

func TestConcurrentAcquisitionHasOneWinner(t *testing.T) {
	state := t.TempDir()
	var wait sync.WaitGroup
	winners := make(chan *Lock, 16)
	for i := 0; i < 16; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if acquired, err := Acquire(state); err == nil {
				winners <- acquired
			} else if !errors.Is(err, ErrLocked) {
				t.Errorf("unexpected acquisition failure: %v", err)
			}
		}()
	}
	wait.Wait()
	close(winners)
	count := 0
	for acquired := range winners {
		count++
		if err := acquired.Release(); err != nil {
			t.Fatal(err)
		}
	}
	if count != 1 {
		t.Fatalf("wanted one winner, got %d", count)
	}
}

func TestLockExcludesOtherProcess(t *testing.T) {
	state := t.TempDir()
	acquired, err := Acquire(state)
	if err != nil {
		t.Fatal(err)
	}
	runHelper := func() error {
		cmd := exec.Command(os.Args[0], "-test.run=TestLockProcessHelper")
		cmd.Env = append(os.Environ(), "HOMEVAULT_LOCK_HELPER=1", "HOMEVAULT_LOCK_STATE="+state)
		return cmd.Run()
	}
	err = runHelper()
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != 23 {
		t.Fatalf("another process bypassed lock: %v", err)
	}
	if err := acquired.Release(); err != nil {
		t.Fatal(err)
	}
	if err := runHelper(); err != nil {
		t.Fatalf("released lock prevented child acquisition: %v", err)
	}
}

func TestLockProcessHelper(t *testing.T) {
	if os.Getenv("HOMEVAULT_LOCK_HELPER") != "1" {
		return
	}
	acquired, err := Acquire(os.Getenv("HOMEVAULT_LOCK_STATE"))
	if errors.Is(err, ErrLocked) {
		os.Exit(23)
	}
	if err != nil {
		os.Exit(24)
	}
	if acquired.Release() != nil {
		os.Exit(25)
	}
	os.Exit(0)
}
