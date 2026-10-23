// Package lock provides an exclusive, cross-process filesystem lock. Locks are
// deliberately never automatically removed, even if their recorded PID is gone.
package lock

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrLocked = errors.New("another HomeVault operation holds the state lock; if it is stale, verify no operation is running before manually removing the lock directory")

type Lock struct {
	path  string
	token string
}

type owner struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Token     string    `json:"token"`
}

func Acquire(stateDir string) (*Lock, error) {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	path := filepath.Join(stateDir, "operation.lock")
	if err := os.Mkdir(path, 0700); err != nil {
		if os.IsExist(err) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("acquire state lock: %w", err)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		_ = os.Remove(path)
		return nil, errors.New("generate lock owner token")
	}
	token := hex.EncodeToString(nonce[:])
	data, _ := json.Marshal(owner{PID: os.Getpid(), StartedAt: time.Now().UTC(), Token: token})
	if err := os.WriteFile(filepath.Join(path, "owner.json"), data, 0600); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("record lock owner: %w", err)
	}
	return &Lock{path: path, token: token}, nil
}

// Release removes only the lock acquired by this object. A failed release leaves
// the lock in place and should be reported to the operator.
func (l *Lock) Release() error {
	if l == nil || l.token == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(l.path, "owner.json"))
	if err != nil {
		return fmt.Errorf("read lock owner during release: %w", err)
	}
	var recorded owner
	if json.Unmarshal(data, &recorded) != nil || recorded.Token != l.token {
		return errors.New("refusing to release a lock with a different owner")
	}
	if err := os.Remove(filepath.Join(l.path, "owner.json")); err != nil {
		return fmt.Errorf("remove lock owner: %w", err)
	}
	if err := os.Remove(l.path); err != nil {
		return fmt.Errorf("release state lock: %w", err)
	}
	l.token = ""
	return nil
}
