// Package crypto provides envelope encryption for tenant secrets.
//
// Each tenant secret (OAuth tokens, BYO OpenAI key, destination configs)
// is encrypted with a per-tenant data key, which itself is encrypted
// ("wrapped") with a KMS master key. This pattern lets us rotate the
// master key without re-encrypting every tenant's data, and lets us
// audit access via KMS CloudTrail.
//
// Implementation note: the actual AWS KMS calls live in cmd/api and use
// the AWS SDK; this package only defines the data shapes and the
// AES-GCM envelope encryption that wraps the data keys.
package crypto

import (
	"context"
	"errors"
)

// Envelope represents a wrapped (encrypted) secret.
//
// Layout: ciphertext = nonce(12) || aesgcm_seal(plaintext, aad=tenant_id)
type Envelope struct {
	KeyID      string // KMS key ARN or alias
	WrappedKey []byte // data key encrypted under KMS
	Ciphertext []byte // secret encrypted under data key
	Algorithm  string // "AES-256-GCM"
}

// EnvelopeCipher is the high-level encrypt/decrypt API.
type EnvelopeCipher interface {
	// Encrypt encrypts plaintext under a fresh data key for tenantID.
	Encrypt(ctx context.Context, tenantID string, plaintext []byte) (*Envelope, error)
	// Decrypt decrypts an Envelope back to plaintext.
	Decrypt(ctx context.Context, tenantID string, env *Envelope) ([]byte, error)
}

// KMSClient is the interface the cmd/api layer implements using AWS SDK.
type KMSClient interface {
	// GenerateDataKey returns a fresh plaintext data key + the same key
	// encrypted under the master key.
	GenerateDataKey(ctx context.Context, keyID string) (plaintext, wrapped []byte, err error)
	// DecryptDataKey unwraps a wrapped data key back to plaintext.
	DecryptDataKey(ctx context.Context, wrapped []byte) (plaintext []byte, err error)
}

// ErrInvalidEnvelope is returned when the envelope layout is wrong.
var ErrInvalidEnvelope = errors.New("invalid envelope")

// ErrDecryptionFailed is returned when AES-GCM authentication fails.
var ErrDecryptionFailed = errors.New("decryption failed (tampered or wrong key)")

// AADForTenant builds the additional authenticated data used for envelope
// encryption. Binding ciphertext to tenant_id prevents envelope-replay
// across tenants.
func AADForTenant(tenantID string) []byte {
	return []byte("niosengine:tenant:" + tenantID)
}
