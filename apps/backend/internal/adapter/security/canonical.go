package security

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
)

// CanonicalMutation returns RFC 8785 (JCS) canonical JSON for the signed
// mutation without its signature. The cross-platform contract always includes
// baseRevision (null for creates) and uses UTC timestamps with milliseconds.
func CanonicalMutation(m domain.SyncMutation) ([]byte, error) {
	value := map[string]any{
		"operationId":     m.OperationID,
		"aggregate":       m.Aggregate,
		"action":          m.Action,
		"aggregateId":     m.AggregateID,
		"baseRevision":    nil,
		"originSessionId": m.OriginSessionID.String(),
		"originActorId":   m.OriginActorID.String(),
		"terminalId":      m.TerminalID.String(),
		"occurredAt":      m.OccurredAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if m.BaseRevision != nil {
		value["baseRevision"] = *m.BaseRevision
	}
	if !json.Valid(m.Payload) {
		return nil, fmt.Errorf("decode payload: invalid JSON")
	}
	value["payload"] = json.RawMessage(m.Payload)
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal signed mutation: %w", err)
	}
	canonical, err := jsoncanonicalizer.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("canonicalize signed mutation: %w", err)
	}
	return canonical, nil
}

func VerifyMutation(publicKey []byte, m domain.SyncMutation) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid terminal public key")
	}
	signature, err := base64.RawStdEncoding.DecodeString(m.Signature)
	if err != nil {
		signature, err = base64.StdEncoding.DecodeString(m.Signature)
	}
	if err != nil {
		signature, err = base64.RawURLEncoding.DecodeString(m.Signature)
	}
	if err != nil {
		signature, err = base64.URLEncoding.DecodeString(m.Signature)
	}
	if err != nil {
		signature, err = hex.DecodeString(m.Signature)
	}
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature encoding")
	}
	message, err := CanonicalMutation(m)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), message, signature) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}
