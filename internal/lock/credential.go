package lock

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// AcquireCredential serializes HomeVault OAuth/config refresh users without
// reentering the backup state lock. The config's parent must be writable.
func AcquireCredential(file string) (*Lock, error) {
	hash := sha256.Sum256([]byte(filepath.Base(file)))
	return Acquire(filepath.Join(filepath.Dir(file), ".homevault-credential-"+hex.EncodeToString(hash[:8])))
}
