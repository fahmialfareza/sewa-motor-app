package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

type OpaqueTokenManager struct{}

func (OpaqueTokenManager) New() (string, []byte, error) {
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", nil, fmt.Errorf("read session token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(rawBytes)
	hash := sha256.Sum256([]byte(raw))
	return raw, hash[:], nil
}

func (OpaqueTokenManager) Hash(raw string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("invalid opaque token")
	}
	hash := sha256.Sum256([]byte(raw))
	return hash[:], nil
}
