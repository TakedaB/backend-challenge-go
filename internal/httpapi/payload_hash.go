package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// hashPayload computes a deterministic SHA-256 hash of the request
// body, used as WagerTransaction.PayloadHash. This is a simplified
// stand-in for the README's canonical-JSON hashing requirement
// (documented as a scope limitation in ARCHITECTURE.md): a byte-for-
// byte identical request produces the same hash, but this does not
// guarantee semantic equivalence across differently-ordered or
// differently-formatted JSON representing the same values.
func hashPayload(req processWagerTransactionRequest) string {
	b, _ := json.Marshal(req)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
