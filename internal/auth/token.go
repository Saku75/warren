package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// APITokenPrefix marks Warren API tokens so leaked credentials are
// recognizable to secret scanners.
const APITokenPrefix = "wrt_"

// newSecret returns a 256-bit random string (base64url, no padding).
func newSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("auth: reading entropy: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken digests a presented token for storage or lookup. Tokens are
// random 256-bit values, so an unsalted SHA-256 is sufficient and allows
// indexed lookup.
func HashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// NewSessionToken returns the cookie value and its storage hash.
func NewSessionToken() (raw string, hash []byte) {
	raw = newSecret()
	return raw, HashToken(raw)
}

// NewAPIToken returns the presentable token (with prefix) and its
// storage hash.
func NewAPIToken() (raw string, hash []byte) {
	raw = APITokenPrefix + newSecret()
	return raw, HashToken(raw)
}

// IsAPIToken reports whether raw is shaped like a Warren API token.
func IsAPIToken(raw string) bool {
	return strings.HasPrefix(raw, APITokenPrefix)
}

// NewLoginCSRF returns a random value for the login form's double-submit
// cookie (sessions carry their own CSRF token once established).
func NewLoginCSRF() string {
	return newSecret()
}
