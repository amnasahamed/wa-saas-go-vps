// JWT minter/parser using stdlib HMAC-SHA256 to avoid an external dep.
//
// Format: header.payload.signature, all base64url.
// Header: {"alg":"HS256","typ":"JWT"}.
// Payload claims: sub (user_id), tid (tenant_id), email, role, iat, exp.

package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// jwtHeader is the static header for HS256.
var jwtHeader = []byte(`{"alg":"HS256","typ":"JWT"}`)

// jwtClaims is the body of a JWT.
type jwtClaims struct {
	Sub  string `json:"sub"`  // user id
	TID  string `json:"tid"`  // tenant id
	Em   string `json:"email"`
	Role string `json:"role"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

// b64URL is a shortcut for base64url-no-padding encoding used in JWT.
var b64URL = base64.RawURLEncoding

// MintToken signs a Principal with the given secret. Expiry defaults to 7d.
func MintToken(p *Principal, secret []byte) (string, error) {
	if p == nil {
		return "", errors.New("nil principal")
	}
	now := time.Now().UTC()
	claims := jwtClaims{
		Sub:  p.UserID,
		TID:  p.TenantID,
		Em:   p.Email,
		Role: p.Role,
		Iat:  now.Unix(),
		Exp:  now.Add(7 * 24 * time.Hour).Unix(),
	}
	header := b64URL.EncodeToString(jwtHeader)
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payload := b64URL.EncodeToString(payloadBytes)
	signing := header + "." + payload
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	sig := b64URL.EncodeToString(mac.Sum(nil))
	return signing + "." + sig, nil
}

// ParseToken verifies the signature and returns the claims.
func ParseToken(token string, secret []byte) (*Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrTokenInvalid
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := b64URL.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return nil, ErrTokenInvalid
	}
	payloadBytes, err := b64URL.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var c jwtClaims
	if err := json.Unmarshal(payloadBytes, &c); err != nil {
		return nil, ErrTokenInvalid
	}
	if c.Exp > 0 && time.Now().UTC().Unix() > c.Exp {
		return nil, ErrTokenInvalid
	}
	return &Principal{
		UserID:   c.Sub,
		TenantID: c.TID,
		Email:    c.Em,
		Role:     c.Role,
	}, nil
}
