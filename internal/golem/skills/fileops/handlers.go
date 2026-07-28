package fileops

import (
	"context"
	"encoding/json"
	"fmt"

	core "github.com/kiosk404/echoryn/pkg/fileops"
)

// HandleRead decodes ReadPayload and returns JSON-encoded *core.ReadResult.
// The outer error is non-nil only on JSON decode failure; all domain errors
// (sandbox denial, file not found, etc.) are encoded in the ReadResult.Error
// field so Hivemind can surface them verbatim to the LLM.
func HandleRead(ctx context.Context, payload []byte, sb *core.Sandbox) (string, error) {
	var p ReadPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("invalid read payload: %w", err)
	}
	r, _ := core.ReadFile(sb, p.Path, p.Offset, p.Limit)
	b, _ := json.Marshal(r)
	return string(b), nil
}

// HandleWrite decodes WritePayload and returns JSON-encoded *core.WriteResult.
func HandleWrite(ctx context.Context, payload []byte, sb *core.Sandbox) (string, error) {
	var p WritePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("invalid write payload: %w", err)
	}
	r, _ := core.WriteFile(sb, p.Path, p.Content)
	b, _ := json.Marshal(r)
	return string(b), nil
}

// HandlePatch decodes PatchPayload and returns JSON-encoded *core.PatchResult.
// MVP supports only the replace mode; V4A patches are deferred to Phase 2.
func HandlePatch(ctx context.Context, payload []byte, sb *core.Sandbox) (string, error) {
	var p PatchPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("invalid patch payload: %w", err)
	}
	r, _ := core.PatchReplace(sb, p.Path, p.OldString, p.NewString, p.ReplaceAll)
	b, _ := json.Marshal(r)
	return string(b), nil
}

// HandleSearch decodes SearchPayload and returns JSON-encoded *core.SearchResult.
func HandleSearch(ctx context.Context, payload []byte, sb *core.Sandbox) (string, error) {
	var p SearchPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("invalid search payload: %w", err)
	}
	r, _ := core.Search(sb, core.SearchOptions{
		Pattern:    p.Pattern,
		Path:       p.Path,
		Target:     p.Target,
		FileGlob:   p.FileGlob,
		Limit:      p.Limit,
		Offset:     p.Offset,
		OutputMode: p.OutputMode,
		Context:    p.Context,
	})
	b, _ := json.Marshal(r)
	return string(b), nil
}
