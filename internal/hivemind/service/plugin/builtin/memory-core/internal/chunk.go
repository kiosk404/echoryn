package internal

import (
	"strings"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/entity"
)

// ChunkMarkdown splits a Markdown document into overlapping chunks.
// The algorithm matches OpenClaw's chunkMarkdown exactly:
//   - maxChars = max(32, tokens * 4)
//   - overlapChars = max(0, overlap * 4)
//   - Lines that exceed maxChars are split into segments
//   - On flush, the last overlapChars are carried into the next chunk
func ChunkMarkdown(content string, cfg entity.ChunkingConfig) []entity.MemoryChunk {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil
	}

	maxChars := max(32, cfg.Tokens*4)
	overlapChars := max(0, cfg.Overlap*4)

	type lineEntry struct {
		line   string
		lineNo int
	}

	var chunks []entity.MemoryChunk
	var current []lineEntry
	currentChars := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		first := current[0]
		last := current[len(current)-1]

		var sb strings.Builder
		for i, entry := range current {
			if i > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(entry.line)
		}

		text := sb.String()
		chunks = append(chunks, entity.MemoryChunk{
			StartLine: first.lineNo,
			EndLine:   last.lineNo,
			Text:      text,
			Hash:      HashText(text),
		})
	}

	carryOverlap := func() {
		if overlapChars <= 0 || len(current) == 0 {
			current = nil
			currentChars = 0
			return
		}
		acc := 0
		var kept []lineEntry
		for i := len(current) - 1; i >= 0; i-- {
			entry := current[i]
			acc += len(entry.line) + 1
			kept = append([]lineEntry{entry}, kept...)
			if acc >= overlapChars {
				break
			}
		}
		current = kept
		currentChars = 0
		for _, entry := range kept {
			currentChars += len(entry.line) + 1
		}
	}

	for i, line := range lines {
		lineNo := i + 1

		// Split long lines into segments of maxChars.
		var segments []string
		if len(line) == 0 {
			segments = append(segments, "")
		} else {
			for start := 0; start < len(line); start += maxChars {
				end := start + maxChars
				if end > len(line) {
					end = len(line)
				}
				segments = append(segments, line[start:end])
			}
		}

		for _, segment := range segments {
			lineSize := len(segment) + 1
			if currentChars+lineSize > maxChars && len(current) > 0 {
				flush()
				carryOverlap()
			}
			current = append(current, lineEntry{line: segment, lineNo: lineNo})
			currentChars += lineSize
		}
	}

	flush()
	return chunks
}
