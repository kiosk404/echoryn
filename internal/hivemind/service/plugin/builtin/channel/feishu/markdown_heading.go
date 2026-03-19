package feishu

import (
	"regexp"
	"strings"
)

// HeadingTransformer handles heading-level optimization for Feishu rendering.
//
// Feishu card markdown renders H1-H3 as very large text that dominates the card.
// This transformer downgrades headings to more reasonable sizes:
//   - H1 → H4
//   - H2~H6 → H5
//
// Downgrade is only applied when the original text contains H1~H3 headings,
// preserving documents that already use appropriate heading levels.
//
// Reference: openclaw-lark's optimizeMarkdownStyle step 2.
type HeadingTransformer struct{}

var (
	hasLargeHeadingRegex = regexp.MustCompile(`(?m)^#{1,3} `)
	// Process H2~H6 first, then H1, to avoid double-transformation.
	h2to6Regex = regexp.MustCompile(`(?m)^#{2,6} (.+)$`)
	h1Regex    = regexp.MustCompile(`(?m)^# (.+)$`)
)

func (h *HeadingTransformer) Transform(text string, mode RenderMode) string {
	if mode != RenderModeCard {
		return text // Only apply to card mode; post mode handles headings separately.
	}

	// Only downgrade if there are H1~H3 in the text.
	if !hasLargeHeadingRegex.MatchString(text) {
		return text
	}

	// Order matters: process H2~H6 first to avoid H1→H4 being caught by H2~H6 regex.
	text = h2to6Regex.ReplaceAllString(text, "##### $1")
	text = h1Regex.ReplaceAllString(text, "#### $1")

	return text
}

// HeadingSpacingTransformer adds visual spacing between consecutive headings.
// In Feishu card rendering, consecutive headings without spacing look cramped.
//
// Reference: openclaw-lark's optimizeMarkdownStyle step 3.
type HeadingSpacingTransformer struct{}

var consecutiveHeadingsRegex = regexp.MustCompile(`(?m)(^#{1,6} .+)\n(#{1,6} )`)

func (h *HeadingSpacingTransformer) Transform(text string, mode RenderMode) string {
	if mode != RenderModeCard {
		return text
	}

	// Insert <br> between consecutive headings for spacing.
	return consecutiveHeadingsRegex.ReplaceAllString(text, "$1\n<br>\n$2")
}

// ExcessiveNewlineTransformer compresses more than 2 consecutive newlines into exactly 2.
// This prevents large empty gaps in rendered output.
//
// Reference: openclaw-lark's optimizeMarkdownStyle step 6.
type ExcessiveNewlineTransformer struct{}

var excessiveNewlineRegex = regexp.MustCompile(`\n{3,}`)

func (e *ExcessiveNewlineTransformer) Transform(text string, _ RenderMode) string {
	return excessiveNewlineRegex.ReplaceAllString(text, "\n\n")
}

// BlockquoteTransformer converts blockquote syntax for card rendering.
// Feishu card markdown does NOT support > blockquote syntax.
// This transformer converts them to visual alternatives:
//   - "> text" → "│ *text*" (indented italic with vertical bar)
//   - ">" (empty) → "│"
type BlockquoteTransformer struct{}

var blockquoteLineRegex = regexp.MustCompile(`(?m)^> ?(.*)$`)

func (b *BlockquoteTransformer) Transform(text string, mode RenderMode) string {
	if mode != RenderModeCard {
		return text // Post mode handles blockquotes in the post builder.
	}

	return blockquoteLineRegex.ReplaceAllStringFunc(text, func(line string) string {
		content := strings.TrimPrefix(line, "> ")
		content = strings.TrimPrefix(content, ">")
		content = strings.TrimSpace(content)
		if content == "" {
			return "│"
		}
		return "│ *" + content + "*"
	})
}

// HorizontalRuleTransformer converts horizontal rules for card rendering.
// Card markdown supports <hr> but Post does not.
type HorizontalRuleTransformer struct{}

var hrRegex = regexp.MustCompile(`(?m)^(---|\*\*\*|___)$`)

func (h *HorizontalRuleTransformer) Transform(text string, mode RenderMode) string {
	if mode != RenderModeCard {
		return text // Post mode handles HR in post builder.
	}
	return hrRegex.ReplaceAllString(text, "---")
}
