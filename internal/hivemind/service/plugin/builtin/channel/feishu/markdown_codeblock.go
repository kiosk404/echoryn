package feishu

import (
	"fmt"
	"regexp"
	"strings"
)

// CodeBlockGuard protects fenced code blocks from being modified by other transformers.
//
// Algorithm:
//  1. Extract: Replace all fenced code blocks (``` ... ```) with unique placeholders.
//  2. Restore: After other transformers run, replace placeholders back with original code blocks.
//
// This is essential because many transformers use regex that could corrupt code block content.
// Reference: openclaw-lark's optimizeMarkdownStyle step 1 & 5.
type CodeBlockGuard struct {
	blocks []string
}

const codeBlockPlaceholder = "\x00CB_"

var fencedCodeBlockRegex = regexp.MustCompile("(?s)```[^`]*```")

// Extract removes all fenced code blocks and replaces them with placeholders.
// Returns the modified text and stores the blocks internally for later restoration.
func (g *CodeBlockGuard) Extract(text string) string {
	g.blocks = nil
	return fencedCodeBlockRegex.ReplaceAllStringFunc(text, func(match string) string {
		idx := len(g.blocks)
		g.blocks = append(g.blocks, match)
		return fmt.Sprintf("%s%d\x00", codeBlockPlaceholder, idx)
	})
}

// Restore replaces all placeholders back with the original code blocks.
// For Card mode, it adds <br> padding around code blocks for visual spacing.
func (g *CodeBlockGuard) Restore(text string, mode RenderMode) string {
	for i, block := range g.blocks {
		placeholder := fmt.Sprintf("%s%d\x00", codeBlockPlaceholder, i)
		var replacement string
		switch mode {
		case RenderModeCard:
			// Card mode: add <br> around code blocks for visual spacing.
			// Reference: openclaw-lark cardVersion >= 2 behavior.
			replacement = "<br>" + block + "<br>"
		default:
			replacement = block
		}
		text = strings.Replace(text, placeholder, replacement, 1)
	}
	return text
}

// HasCodeBlocks returns true if any code blocks were extracted.
func (g *CodeBlockGuard) HasCodeBlocks() bool {
	return len(g.blocks) > 0
}

// CodeBlockLangs returns the languages of all extracted code blocks.
func (g *CodeBlockGuard) CodeBlockLangs() []string {
	var langs []string
	for _, block := range g.blocks {
		// Extract language from opening fence: ```lang
		if idx := strings.Index(block, "\n"); idx > 3 {
			lang := strings.TrimSpace(block[3:idx])
			if lang != "" {
				langs = append(langs, lang)
			}
		}
	}
	return langs
}
