package internal

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashText returns the SHA256 hash of the given text.
func HashText(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}
