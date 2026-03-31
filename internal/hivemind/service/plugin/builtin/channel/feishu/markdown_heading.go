package feishu

import (
	"regexp"
	"strings"
)

// InlineHeadingFixTransformer fixes inline headings that are not at the start of a line.
// LLM sometimes generates headings like "text。## Title" instead of proper line-start headings.
// This transformer moves such inline headings to a new line so subsequent transformers can handle them.
//
// Example:
//   - "结果如下。## 标题" → "结果如下。\n## 标题"
type InlineHeadingFixTransformer struct{}

var (
	// Match heading syntax preceded by punctuation marks (common LLM output pattern).
	// Only matches when heading follows a punctuation character, not any character,
	// to avoid breaking properly formatted headings at line start.
	// Captures: $1 = punctuation, $2 = heading markers, $3 = space, $4 = title text.
	inlineHeadingRegex = regexp.MustCompile(`([。，；：！？,.:;!?])(#{1,6})(\s)([^\n]+)`)
)

func (i *InlineHeadingFixTransformer) Transform(text string, mode RenderMode) string {
	if mode != RenderModeCard {
		return text
	}
	// Move inline headings to a new line.
	// "text。## Title" → "text。\n## Title"
	return inlineHeadingRegex.ReplaceAllString(text, "$1\n$2$3$4")
}

// HeadingTransformer handles heading-level optimization for Feishu rendering.
//
// Feishu card markdown only supports H1~H4 for title rendering
// H5 and H6 are Not rendered as heading - they appear as raw text with "#####" visible
// This transformer maps headings to Feishu-compatible levels:
//   - H1 → H3 (visually distinct top-level heading)
//   - H2 -> H4 (secondary heading)
//   - H3 ~ H6 (Feishu doesn't render H5/H6 so use bold as fallback)
//
// Downgrade is only applied when the original text contains H1~H3 headings,
// preserving documents that already use appropriate heading levels.
//
// Reference: openclaw-lark's optimizeMarkdownStyle step 2.
type HeadingTransformer struct{}

var (
	hasLargeHeadingRegex = regexp.MustCompile(`(?m)^#{1,3} `)
	hasH5H6Regex         = regexp.MustCompile(`(?m)^#{5,6} `)
	// Process from H3~H6 first (convert to bold), then H2, then H1.
	// This avoids double-transformation issues.
	h3to6Regex = regexp.MustCompile(`(?m)^#{3,6} (.+)$`)
	h5to6Regex = regexp.MustCompile(`(?m)^#{5,6} (.+)$`)
	h2Regex    = regexp.MustCompile(`(?m)^## (.+)$`)
	h1Regex    = regexp.MustCompile(`(?m)^# (.+)$`)
)

func (h *HeadingTransformer) Transform(text string, mode RenderMode) string {
	if mode != RenderModeCard {
		return text // Only apply to card mode; post mode handles headings separately.
	}

	hasLargeHeadings := hasLargeHeadingRegex.MatchString(text)
	hasH5H6 := hasH5H6Regex.MatchString(text)

	if !hasLargeHeadings && !hasH5H6 {
		return text // Only H4 headings present, which Feishu supports natively.
	}

	if hasLargeHeadings {
		// Full downgrade: H1~H3 present, shift everything down.
		// H3~H6 → bold text (Feishu card doesn't render H5/H6 at all).
		text = h3to6Regex.ReplaceAllString(text, "**$1**")
		// H2 → H4.
		text = h2Regex.ReplaceAllString(text, "#### $1")
		// H1 → H3.
		text = h1Regex.ReplaceAllString(text, "### $1")
	} else {
		// No H1~H3, but has H5/H6 that Feishu can't render.
		// Only convert H5~H6 → bold text, leave H4 as-is.
		text = h5to6Regex.ReplaceAllString(text, "**$1**")
	}

	return text
}

// HeadingSpacingTransformer ensures proper spacing between consecutive headings
// and between headings and bold-title lines (which represent downgraded H3~H6).
//
// Uses a single blank line (\n\n) as separator — this is sufficient for Feishu
// card rendering. The previous approach used \n<br>\n which produced excessive
// vertical whitespace (3 lines of spacing instead of 1).
//
// Reference: openclaw-lark's optimizeMarkdownStyle step 3.
type HeadingSpacingTransformer struct{}

var (
	// Match two consecutive heading/bold lines separated by a single \n.
	consecutiveHeadingsRegex = regexp.MustCompile(`(?m)(^#{1,4} .+)\n(#{1,4} )`)
	headingThenBoldRegex     = regexp.MustCompile(`(?m)(^#{1,4} .+)\n(\*\*.+\*\*)$`)
	boldThenHeadingRegex     = regexp.MustCompile(`(?m)(^\*\*.+\*\*)\n(#{1,4} )`)
)

func (h *HeadingSpacingTransformer) Transform(text string, mode RenderMode) string {
	if mode != RenderModeCard {
		return text
	}

	// Insert a blank line between consecutive heading/bold lines for spacing.
	text = consecutiveHeadingsRegex.ReplaceAllString(text, "$1\n\n$2")
	text = headingThenBoldRegex.ReplaceAllString(text, "$1\n\n$2")
	text = boldThenHeadingRegex.ReplaceAllString(text, "$1\n\n$2")

	return text
}

// ExcessiveNewlineTransformer compresses redundant vertical whitespace:
//   - More than 2 consecutive newlines → exactly 2 (\n\n)
//   - Consecutive <br> tags with newlines (e.g. \n<br>\n<br>\n) → single \n\n
//   - Trailing <br> before headings → removed (headings have their own spacing)
//
// Reference: openclaw-lark's optimizeMarkdownStyle step 6.
type ExcessiveNewlineTransformer struct{}

var (
	excessiveNewlineRegex = regexp.MustCompile(`\n{3,}`)
	// Match patterns like \n<br>\n, \n<br>\n<br>\n, etc.
	excessiveBrRegex = regexp.MustCompile(`(\n\s*<br>\s*){2,}`)
	// Single <br> surrounded by blank lines → collapse to \n\n.
	brWithBlankLines = regexp.MustCompile(`\n\n\s*<br>\s*\n`)
)

func (e *ExcessiveNewlineTransformer) Transform(text string, _ RenderMode) string {
	// Collapse consecutive <br> blocks into a single blank line.
	text = excessiveBrRegex.ReplaceAllString(text, "\n\n")
	// Remove <br> that's already between blank lines (redundant spacing).
	text = brWithBlankLines.ReplaceAllString(text, "\n\n")
	// Collapse excessive newlines.
	text = excessiveNewlineRegex.ReplaceAllString(text, "\n\n")
	return text
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
