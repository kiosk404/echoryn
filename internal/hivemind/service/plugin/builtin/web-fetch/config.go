// Package webfetch implements the "web-fetch" built-in plugin.
//
// This plugin provides a lightweight HTTP fetch + readable content extraction
// tool for agents. It fetches a URL, extracts readable content from HTML
// (converting to markdown or plain text), and returns the result.
//
// Aligned with OpenClaw's web_fetch tool:
//   - DOM-level Readability extraction (go-readability, port of @mozilla/readability)
//   - Firecrawl API fallback when Readability fails
//   - Full hidden-element sanitization via DOM walk (prompt injection defense)
//   - SSRF protection (private/internal IP blocking + redirect re-check)
//   - externalContent marking + wrapWebContent security boundary
//   - Invisible Unicode stripping
//   - Response size limiting and truncation
//   - In-memory caching with configurable TTL
//   - HTML nesting depth guard
//   - Cloudflare Markdown for Agents support
//
// Registered capabilities:
//   - Tool: "web_fetch" — fetch and extract readable content from a URL
package webfetch

// Config holds the web-fetch plugin configuration.
type Config struct {
	// Enabled controls whether the plugin is active.
	Enabled bool `json:"enabled"`

	// MaxChars is the default maximum characters to return.
	// Agent can override per-call via the maxChars parameter (clamped to MaxCharsCap).
	// Default: 50000.
	MaxChars int `json:"max_chars"`

	// MaxCharsCap is the absolute upper bound for maxChars.
	// Default: 50000.
	MaxCharsCap int `json:"max_chars_cap"`

	// MaxResponseBytes caps the downloaded response body before parsing.
	// Range: [32000, 10000000]. Default: 2000000 (2 MB).
	MaxResponseBytes int `json:"max_response_bytes"`

	// TimeoutSeconds is the HTTP request timeout in seconds.
	// Default: 30.
	TimeoutSeconds int `json:"timeout_seconds"`

	// CacheTTLMinutes controls how long fetch results are cached.
	// Default: 15.
	CacheTTLMinutes int `json:"cache_ttl_minutes"`

	// MaxRedirects limits HTTP redirect following.
	// Default: 3.
	MaxRedirects int `json:"max_redirects"`

	// UserAgent is the User-Agent header sent with requests.
	// Default: Chrome-like UA string.
	UserAgent string `json:"user_agent"`

	// Readability controls whether Readability-based extraction is enabled.
	// Default: true.
	Readability bool `json:"readability"`

	// Firecrawl holds optional Firecrawl API fallback configuration.
	// When configured, Firecrawl is used as fallback when direct fetch or
	// Readability extraction fails (matching OpenClaw's 3-tier strategy).
	Firecrawl *FirecrawlConfig `json:"firecrawl,omitempty"`
}

const (
	defaultMaxChars         = 50_000
	defaultMaxCharsCap      = 50_000
	defaultMaxResponseBytes = 2_000_000
	defaultTimeoutSeconds   = 30
	defaultCacheTTLMinutes  = 15
	defaultMaxRedirects     = 3
	defaultUserAgent        = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7_2) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
	minMaxResponseBytes     = 32_000
	maxMaxResponseBytes     = 10_000_000

	defaultFirecrawlMaxAgeMs       int64 = 172_800_000 // 48 hours
	defaultFirecrawlTimeoutSeconds       = 60
)

// DefaultConfig returns sensible defaults for the web-fetch plugin.
func DefaultConfig() *Config {
	return &Config{
		Enabled:          false,
		MaxChars:         defaultMaxChars,
		MaxCharsCap:      defaultMaxCharsCap,
		MaxResponseBytes: defaultMaxResponseBytes,
		TimeoutSeconds:   defaultTimeoutSeconds,
		CacheTTLMinutes:  defaultCacheTTLMinutes,
		MaxRedirects:     defaultMaxRedirects,
		UserAgent:        defaultUserAgent,
		Readability:      true,
	}
}
