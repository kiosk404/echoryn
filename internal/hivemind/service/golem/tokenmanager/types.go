package tokenmanager

import (
	"context"
	"time"
)

// TokenManager manages the lifecycle of Admin Token and Bootstrap Tokens.
type TokenManager interface {
	// CreateToken creates a new Bootstrap Token.
	CreateToken(ctx context.Context, ttl time.Duration, maxUsages int32, description string, labels map[string]string) (token string, info *BootstrapToken, err error)

	// ListTokens returns all Bootstrap Tokens.
	ListTokens(ctx context.Context) ([]*BootstrapToken, error)

	// DeleteToken removes a Bootstrap Token by its ID.
	DeleteToken(ctx context.Context, tokenID string) error

	// ValidateBootstrapToken validates a full token string (id.secret) and returns the token info.
	ValidateBootstrapToken(ctx context.Context, token string) (*BootstrapToken, error)

	// ConsumeToken decrements the usage count of a Bootstrap Token.
	ConsumeToken(ctx context.Context, tokenID string) error

	// ValidateAdminToken checks whether a full token string is the valid Admin Token.
	ValidateAdminToken(token string) bool

	// AdminToken returns the full admin token string (only available during the process lifetime).
	AdminToken() string

	// Start loads persisted tokens and starts background cleanup.
	Start(ctx context.Context) error

	// Stop persists state and releases resources.
	Stop(ctx context.Context) error
}

// BootstrapToken represents a short-lived token for Golem node joining.
type BootstrapToken struct {
	ID          string            // 6-char ID
	SecretHash  string            // SHA-256 hash of the secret
	ExpiresAt   time.Time         // Expiration time
	Usages      int32             // Remaining uses (0 == unlimited)
	MaxUsages   int32             // Original max uses
	Description string            // Human-readable description
	CreatedAt   time.Time         // Creation time
	CreatedBy   string            // Creator identity
	Labels      map[string]string // Arbitrary labels
}

// IsExpired returns true if the token has expired.
func (t *BootstrapToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsExhausted returns true if the token's usage count is exhausted.
func (t *BootstrapToken) IsExhausted() bool {
	return t.MaxUsages > 0 && t.Usages <= 0
}
