// Package zerotrust provides Zero-Trust service-to-service cryptographic authentication,
// ephemeral token issuance, and decentralized policy enforcement.
package zerotrust

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	// ErrInvalidToken indicates malformed token structure, corrupt encoding, or schema mismatch.
	ErrInvalidToken = errors.New("invalid security token")

	// ErrTokenExpired indicates that the ephemeral token has exceeded its validity window.
	ErrTokenExpired = errors.New("security token has expired")

	// ErrSignatureMismatch denotes cryptographic authentication failure during HMAC verification.
	ErrSignatureMismatch = errors.New("token signature mismatch")

	// ErrAudienceMismatch indicates that the token was intended for a different recipient service.
	ErrAudienceMismatch = errors.New("audience mismatch")

	// ErrReplayDetected indicates that the token nonce has already been consumed, signaling a replay attack.
	ErrReplayDetected = errors.New("token replay detected")
)

// Claims represents the cryptographic payload declaring caller identity, audience, lifecycle bounds, and anti-replay nonce.
type Claims struct {
	// Issuer identifies the initiating service that produced the token.
	Issuer string `json:"iss"`

	// Audience identifies the intended downstream recipient service.
	Audience string `json:"aud"`

	// IssuedAt is the Unix timestamp marking when the token was created.
	IssuedAt int64 `json:"iat"`

	// ExpiresAt is the Unix timestamp after which the token is considered invalid.
	ExpiresAt int64 `json:"exp"`

	// Nonce is a cryptographically random unique identifier used to protect against replay attacks.
	Nonce string `json:"nonce"`
}

// TokenManager handles generation, HMAC signing, and constant-time validation of ephemeral service tokens.
//
// Note: Anti-replay protection currently relies on in-memory storage.
// In a horizontally scaled distributed system, a centralized cache (e.g., Redis)
type TokenManager struct {
	mu       sync.Mutex
	stopOnce sync.Once
	seen     map[string]time.Time
	stopChan chan struct{}
	secret   []byte
	ttl      time.Duration
}

// NewTokenManager creates an initialized TokenManager configured with a shared secret key and expiration TTL.
func NewTokenManager(secret []byte, ttl time.Duration) *TokenManager {
	tm := &TokenManager{
		seen:     make(map[string]time.Time),
		stopChan: make(chan struct{}),
		secret:   secret,
		ttl:      ttl,
	}

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				tm.cleanup()
			case <-tm.stopChan:
				return
			}
		}
	}()

	return tm
}

// Issue generates a signed, URL-safe base64 token asserting service identity and intended target.
func (tm *TokenManager) Issue(issuer, audience string) (string, error) {
	now := time.Now()

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", err
	}

	claims := Claims{
		Issuer:    issuer,
		Audience:  audience,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(tm.ttl).Unix(),
		Nonce:     hex.EncodeToString(nonceBytes),
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := tm.sign(encodedPayload)
	encodedSignature := base64.RawURLEncoding.EncodeToString(signature)

	return encodedPayload + "." + encodedSignature, nil
}

// Verify validates token format, cryptographic signature, expiration, and audience targeting.
// It uses constant-time comparison to protect against timing attacks.
func (tm *TokenManager) Verify(tokenString, expectedAudience string) (*Claims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidToken
	}

	encodedPayload, encodedSignature := parts[0], parts[1]

	rawSignature, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil {
		return nil, ErrInvalidToken
	}

	expectedSignature := tm.sign(encodedPayload)
	if subtle.ConstantTimeCompare(rawSignature, expectedSignature) != 1 {
		return nil, ErrSignatureMismatch
	}

	rawPayload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(rawPayload, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if claims.Nonce == "" {
		return nil, ErrInvalidToken
	}

	if claims.ExpiresAt < time.Now().Unix() {
		return nil, ErrTokenExpired
	}

	if expectedAudience != "" && claims.Audience != expectedAudience {
		return nil, ErrAudienceMismatch
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.seen[claims.Nonce]; exists {
		return nil, ErrReplayDetected
	}

	tm.seen[claims.Nonce] = time.Unix(claims.ExpiresAt, 0)
	return &claims, nil
}

// Stop terminates the background garbage collection goroutine.
// It must be called to prevent goroutine leaks when the TokenManager is no longer needed.
func (tm *TokenManager) Stop() {
	tm.stopOnce.Do(func() {
		close(tm.stopChan)
	})
}

// sign generates an HMAC-SHA256 signature for the provided payload string.
func (tm *TokenManager) sign(data string) []byte {
	hash := hmac.New(sha256.New, tm.secret)
	hash.Write([]byte(data))
	return hash.Sum(nil)
}

// cleanup iterates through the stored nonces and removes those that have expired,
// preventing the in-memory replay cache from growing indefinitely.
func (tm *TokenManager) cleanup() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	for nonce, expiresAt := range tm.seen {
		if now.After(expiresAt) {
			delete(tm.seen, nonce)
		}
	}
}
