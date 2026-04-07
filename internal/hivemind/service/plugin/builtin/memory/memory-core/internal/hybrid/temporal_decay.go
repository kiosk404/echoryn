package hybrid

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TemporalDecayConfig configures time-based score decay.
// Newer documents score higher; older documents are penalized with exponential decay.
type TemporalDecayConfig struct {
	// Enabled controls whether temporal decay is applied. Default: false (opt-in).
	Enabled bool `json:"enabled"`

	// HalfLifeDays is the number of days after which a document's score is halved.
	// Default: 30.
	HalfLifeDays float64 `json:"half_life_days"`
}

// DefaultTemporalDecayConfig returns the default temporal decay configuration (disabled, 30-day half-life).
func DefaultTemporalDecayConfig() TemporalDecayConfig {
	return TemporalDecayConfig{
		Enabled:      false,
		HalfLifeDays: 30,
	}
}

// datedMemoryPathRE matches memory/YYYY-MM-DD.md paths.
var datedMemoryPathRE = regexp.MustCompile(`(?:^|/)memory/(\d{4})-(\d{2})-(\d{2})\.md$`)

const dayMs = 24 * 60 * 60 * 1000 // milliseconds in a day

// toDecayLambda converts a half-life in days to the exponential decay constant λ.
// λ = ln(2) / halfLifeDays
func toDecayLambda(halfLifeDays float64) float64 {
	if halfLifeDays <= 0 || math.IsInf(halfLifeDays, 0) || math.IsNaN(halfLifeDays) {
		return 0
	}
	return math.Ln2 / halfLifeDays
}

// calculateTemporalDecayMultiplier returns e^(-λ × ageInDays).
func calculateTemporalDecayMultiplier(ageInDays, halfLifeDays float64) float64 {
	lambda := toDecayLambda(halfLifeDays)
	clampedAge := math.Max(0, ageInDays)
	if lambda <= 0 || math.IsInf(clampedAge, 0) || math.IsNaN(clampedAge) {
		return 1
	}
	return math.Exp(-lambda * clampedAge)
}

// parseMemoryDateFromPath extracts a date from memory/YYYY-MM-DD.md paths.
// Returns zero time if the path doesn't match.
func parseMemoryDateFromPath(filePath string) (time.Time, bool) {
	normalized := strings.ReplaceAll(filePath, "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")

	match := datedMemoryPathRE.FindStringSubmatch(normalized)
	if match == nil {
		return time.Time{}, false
	}

	year, _ := strconv.Atoi(match[1])
	month, _ := strconv.Atoi(match[2])
	day, _ := strconv.Atoi(match[3])

	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

	// Validate the parsed date components match (catches invalid dates like Feb 30).
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return time.Time{}, false
	}

	return t, true
}

// isEvergreenMemoryPath checks if a path is an "evergreen" memory file that should not decay.
// Evergreen files: MEMORY.md, memory.md, and non-dated files under memory/.
func isEvergreenMemoryPath(filePath string) bool {
	normalized := strings.ReplaceAll(filePath, "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")

	if normalized == "MEMORY.md" || normalized == "memory.md" {
		return true
	}
	if !strings.HasPrefix(normalized, "memory/") {
		return false
	}
	// Under memory/ but NOT a dated file → evergreen.
	return !datedMemoryPathRE.MatchString(normalized)
}

// extractTimestamp determines the age-relevant timestamp for a file.
// Priority: 1) date from path (memory/YYYY-MM-DD.md), 2) file mtime, 3) nil (no decay).
// Evergreen memory files return nil (no decay applied).
func extractTimestamp(filePath, source, workspaceDir string) *time.Time {
	// Try to parse date from path.
	if t, ok := parseMemoryDateFromPath(filePath); ok {
		return &t
	}

	// Evergreen memory files don't decay.
	if source == "memory" && isEvergreenMemoryPath(filePath) {
		return nil
	}

	if workspaceDir == "" {
		return nil
	}

	// Fall back to file modification time.
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(workspaceDir, filePath)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil
	}

	mtime := info.ModTime()
	return &mtime
}

// ApplyTemporalDecay applies time-based exponential decay to search result scores.
// Results from newer files keep their scores; older files get penalized.
// Evergreen files (MEMORY.md, non-dated memory files) are exempt from decay.
//
// Matches OpenClaw's applyTemporalDecayToHybridResults.
func ApplyTemporalDecay(results []HybridResult, cfg TemporalDecayConfig, workspaceDir string, now time.Time) []HybridResult {
	if !cfg.Enabled || len(results) == 0 {
		return results
	}

	nowMs := float64(now.UnixMilli())

	// Cache timestamps by source:path to avoid redundant stat calls.
	timestampCache := make(map[string]*time.Time)

	out := make([]HybridResult, len(results))
	for i, r := range results {
		out[i] = r

		cacheKey := string(r.Source) + ":" + r.Path
		ts, cached := timestampCache[cacheKey]
		if !cached {
			ts = extractTimestamp(r.Path, string(r.Source), workspaceDir)
			timestampCache[cacheKey] = ts
		}

		if ts == nil {
			continue // Evergreen or unknown — no decay.
		}

		ageMs := math.Max(0, nowMs-float64(ts.UnixMilli()))
		ageInDays := ageMs / float64(dayMs)
		multiplier := calculateTemporalDecayMultiplier(ageInDays, cfg.HalfLifeDays)
		out[i].Score = r.Score * multiplier
	}

	return out
}
