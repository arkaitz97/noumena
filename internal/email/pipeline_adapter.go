package email

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// IdempotencyKey constructs a deterministic key from the template id,
// recipient, and a stable per-call payload. SHA-256 keeps the key length
// fixed so it fits cleanly in the email_send_log.idempotency_key TEXT
// column without surprises in indexes.
func IdempotencyKey(template, to string, payload ...string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s", template, to)
	for _, p := range payload {
		fmt.Fprintf(h, "|%s", p)
	}
	return template + ":" + hex.EncodeToString(h.Sum(nil))[:32]
}
