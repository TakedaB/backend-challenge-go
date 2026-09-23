package idgen

// Package idgen generates identifiers for new aggregates (wallets,
// ledger entries, wager transactions). It exists so no call site
// needs to depend on a UUID library directly, and so tests can swap
// in a deterministic generator later if needed.

import (
	"crypto/rand"
	"fmt"
)

// New returns a random UUID v4 string.
func New() string {
	b := make([]byte, 16)
	// crypto/rand.Read on this size never returns a short read without
	// an error, and an error here would mean the OS's CSPRNG is
	// broken — not something the caller can recover from, so panicking
	// is preferable to silently returning a low-quality or empty id.
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("idgen: reading random bytes: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
