package tokenmanager

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// manager is the in-memory implementation of TokenManager.
type manager struct {
	cfg *Config

	mu     sync.RWMutex
	tokens map[string]*BootstrapToken // key: token ID

	// Admin Token
	adminTokenFull string // full "ecr-admin.<secret>" (kept in memory only)
	adminTokenHash string // SHA-256 hash for validation

	cancel  context.CancelFunc
	stopped chan struct{}
}

var _ TokenManager = (*manager)(nil)

// newManager creates a new token manager and generates the Admin Token.
func newManager(cfg *Config) (*manager, error) {
	m := &manager{
		cfg:     cfg,
		tokens:  make(map[string]*BootstrapToken),
		stopped: make(chan struct{}),
	}

	// Generate Admin Token (ecr-admin.<16-char-secret>).
	secret, err := generateRandomHex(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate admin token: %w", err)
	}
	m.adminTokenFull = "ecr-admin." + secret
	m.adminTokenHash = hashSecret("ecr-admin", secret)

	return m, nil
}

// AdminToken returns the full admin token string.
func (m *manager) AdminToken() string {
	return m.adminTokenFull
}

// ValidateAdminToken checks whether a full token string is the valid Admin Token.
func (m *manager) ValidateAdminToken(token string) bool {
	id, secret, err := parseToken(token)
	if err != nil {
		return false
	}
	if id != "ecr-admin" {
		return false
	}
	return m.adminTokenHash == hashSecret(id, secret)
}

// CreateToken creates a new Bootstrap Token.
func (m *manager) CreateToken(
	ctx context.Context,
	ttl time.Duration,
	maxUsages int32,
	description string,
	labels map[string]string,
) (string, *BootstrapToken, error) {
	// Clamp TTL.
	if ttl <= 0 {
		ttl = m.cfg.DefaultTTL
	}
	if ttl > m.cfg.MaxTTL {
		ttl = m.cfg.MaxTTL
	}

	id, err := generateRandomHex(6)
	if err != nil {
		return "", nil, fmt.Errorf("generate token id: %w", err)
	}
	secret, err := generateRandomHex(16)
	if err != nil {
		return "", nil, fmt.Errorf("generate token secret: %w", err)
	}

	now := time.Now()
	bt := &BootstrapToken{
		ID:          id,
		SecretHash:  hashSecret(id, secret),
		ExpiresAt:   now.Add(ttl),
		Usages:      maxUsages,
		MaxUsages:   maxUsages,
		Description: description,
		CreatedAt:   now,
		Labels:      labels,
	}

	m.mu.Lock()
	m.tokens[id] = bt
	m.mu.Unlock()

	fullToken := id + "." + secret
	logger.Info("[TokenManager] created bootstrap token %s (ttl=%s, maxUsages=%d)", id, ttl, maxUsages)
	return fullToken, bt, nil
}

// ListTokens returns all Bootstrap Tokens (including expired ones; caller can filter).
func (m *manager) ListTokens(ctx context.Context) ([]*BootstrapToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*BootstrapToken, 0, len(m.tokens))
	for _, bt := range m.tokens {
		result = append(result, bt)
	}
	return result, nil
}

// DeleteToken removes a Bootstrap Token by its ID.
func (m *manager) DeleteToken(ctx context.Context, tokenID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tokens[tokenID]; !ok {
		return fmt.Errorf("token %s not found", tokenID)
	}
	delete(m.tokens, tokenID)
	logger.Info("[TokenManager] deleted bootstrap token %s", tokenID)
	return nil
}

// ValidateBootstrapToken validates a full token string (id.secret).
func (m *manager) ValidateBootstrapToken(ctx context.Context, token string) (*BootstrapToken, error) {
	id, secret, err := parseToken(token)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	bt, ok := m.tokens[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("token not found: %s", id)
	}

	if bt.IsExpired() {
		return nil, fmt.Errorf("token expired: %s", id)
	}

	if bt.SecretHash != hashSecret(id, secret) {
		return nil, fmt.Errorf("invalid token secret")
	}

	if bt.IsExhausted() {
		return nil, fmt.Errorf("token usage exhausted: %s", id)
	}

	return bt, nil
}

// ConsumeToken decrements the usage count.
func (m *manager) ConsumeToken(ctx context.Context, tokenID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bt, ok := m.tokens[tokenID]
	if !ok {
		return fmt.Errorf("token not found: %s", tokenID)
	}

	if bt.MaxUsages > 0 {
		bt.Usages--
		logger.Info("[TokenManager] consumed token %s (remaining=%d)", tokenID, bt.Usages)
	}
	return nil
}

// Start loads persisted tokens and starts the cleanup loop.
func (m *manager) Start(ctx context.Context) error {
	cleanupCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	go m.cleanupLoop(cleanupCtx)

	logger.Info("[TokenManager] started (admin_token=%s)", maskToken(m.adminTokenFull))
	return nil
}

// Stop stops the cleanup loop.
func (m *manager) Stop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	select {
	case <-m.stopped:
	case <-time.After(3 * time.Second):
	}
	logger.Info("[TokenManager] stopped")
	return nil
}

// cleanupLoop periodically removes expired and exhausted tokens.
func (m *manager) cleanupLoop(ctx context.Context) {
	defer close(m.stopped)

	ticker := time.NewTicker(m.cfg.CleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *manager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var removed int
	for id, bt := range m.tokens {
		if bt.IsExpired() || bt.IsExhausted() {
			delete(m.tokens, id)
			removed++
		}
	}
	if removed > 0 {
		logger.Info("[TokenManager] cleanup: removed %d expired/exhausted tokens", removed)
	}
}

// --- helpers ---

// generateRandomHex generates a random hex string of the given byte-length
// (output string length = length characters, using length/2 random bytes).
func generateRandomHex(length int) (string, error) {
	byteLen := (length + 1) / 2
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:length], nil
}

// hashSecret computes SHA-256("<id>.<secret>").
func hashSecret(id, secret string) string {
	h := sha256.Sum256([]byte(id + "." + secret))
	return hex.EncodeToString(h[:])
}

// parseToken splits "id.secret" into its two parts.
func parseToken(token string) (id, secret string, err error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid token format: expected <id>.<secret>")
	}
	return parts[0], parts[1], nil
}

// maskToken masks a token for safe logging: shows first 4 chars of secret.
func maskToken(token string) string {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "***"
	}
	secret := parts[1]
	if len(secret) > 4 {
		secret = secret[:4] + "****"
	}
	return parts[0] + "." + secret
}
