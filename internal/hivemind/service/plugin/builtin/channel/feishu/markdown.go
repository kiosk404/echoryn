package feishu

import (
	"context"
	"regexp"
	"strings"
)

// MarkdownRenderer is the high-level entry point for converting Markdown to Feishu formats.
// It orchestrates the CodeBlockGuard, Pipeline, and output builders.
//
// Architecture overview:
//
//	Input Markdown
//	     │
//	     ▼
//	┌─────────────────┐
//	│ CodeBlockGuard   │  ← Extract code blocks (protect from regex)
//	│   .Extract()     │
//	└────────┬────────┘
//	         │
//	         ▼
//	┌─────────────────┐
//	│   Pipeline       │  ← Chain of Transformers (table, heading, image, etc.)
//	│  .Transform()    │
//	└────────┬────────┘
//	         │
//	         ▼
//	┌─────────────────┐
//	│ CodeBlockGuard   │  ← Restore code blocks
//	│   .Restore()     │
//	└────────┬────────┘
//	         │
//	    ┌────┴────┐
//	    ▼         ▼
//	 Post       Card
//	Builder    Builder
type MarkdownRenderer struct {
	cardPipeline *Pipeline
	postPipeline *Pipeline
	imageXformer *ImageTransformer // Kept separately for dependency injection.
}

// NewMarkdownRenderer creates a MarkdownRenderer with the given token provider and domain.
// The token provider is used by the ImageTransformer to upload external images.
func NewMarkdownRenderer(getToken func(ctx context.Context) (string, error), domain DomainType) *MarkdownRenderer {
	imgXformer := NewImageTransformer(getToken, domain)

	// Card pipeline: all transformers that optimize for card rendering.
	cardPipeline := NewPipeline(
		&TableTransformer{},
		&HeadingTransformer{},
		&HeadingSpacingTransformer{},
		&BlockquoteTransformer{},
		imgXformer,
		&StripInvalidImages{},
		&ExcessiveNewlineTransformer{},
	)

	// Post pipeline: transformers for post (rich text) rendering.
	postPipeline := NewPipeline(
		&TableTransformer{},
		&ExcessiveNewlineTransformer{},
	)

	return &MarkdownRenderer{
		cardPipeline: cardPipeline,
		postPipeline: postPipeline,
		imageXformer: imgXformer,
	}
}

// RenderToPost converts Markdown text to Feishu Post (rich text) format.
// This is used for plain text conversations without complex elements.
func (r *MarkdownRenderer) RenderToPost(markdown string) *PostContent {
	// Step 1: Protect code blocks.
	guard := &CodeBlockGuard{}
	text := guard.Extract(markdown)

	// Step 2: Run post pipeline.
	text = r.postPipeline.Transform(text, RenderModePost)

	// Step 3: Restore code blocks.
	text = guard.Restore(text, RenderModePost)

	// Step 4: Build post structure.
	return buildPost(text)
}

// RenderToCard converts Markdown text to Feishu Interactive Card format.
// This is used when content contains code blocks, tables, or other rich elements.
func (r *MarkdownRenderer) RenderToCard(markdown string) map[string]interface{} {
	// Step 1: Protect code blocks.
	guard := &CodeBlockGuard{}
	text := guard.Extract(markdown)

	// Step 2: Run card pipeline.
	text = r.cardPipeline.Transform(text, RenderModeCard)

	// Step 3: Restore code blocks (with <br> spacing for card).
	text = guard.Restore(text, RenderModeCard)

	// Step 4: Extract title and build card structure.
	cardContent := extractCardContent(text)
	return buildCard(cardContent)
}

// NeedsCardRendering determines if the markdown content requires card rendering.
// Returns true if the content contains elements that render poorly in Post format.
//
// Reference: openclaw-lark's shouldUseCard function.
func NeedsCardRendering(markdown string) bool {
	// Code blocks require card for proper syntax highlighting.
	if strings.Contains(markdown, "```") {
		return true
	}
	// Tables require card for native table rendering.
	if tableBlockRegex.MatchString(markdown) {
		return true
	}
	// Images require card for inline rendering.
	if imageMarkdownRegex.MatchString(markdown) {
		return true
	}
	return false
}

// --- Post Builder ---

// Regular expressions for post parsing.
var (
	postHeadingRegex = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	postListRegex    = regexp.MustCompile(`^(\s*)[-*+]\s+(.+)$`)
	postNumListRegex = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.+)$`)
	postLinkRegex    = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// buildPost converts processed markdown text into Feishu Post structure.
// This is a line-by-line parser that handles:
//   - Headings → title extraction + bold text
//   - Code blocks → formatted code display
//   - Lists → bullet/numbered items
//   - Blockquotes → indented with vertical bar
//   - Links → clickable text blocks
//   - Horizontal rules → visual separator
//   - Inline styles → bold/italic marker removal (Post doesn't support styles)
func buildPost(markdown string) *PostContent {
	lines := strings.Split(markdown, "\n")

	var title string
	var paragraphs [][]PostBlock
	var currentParagraph []PostBlock
	var inCodeBlock bool
	var codeBlockContent strings.Builder
	var codeBlockLang string

	flushParagraph := func() {
		if len(currentParagraph) > 0 {
			paragraphs = append(paragraphs, currentParagraph)
			currentParagraph = nil
		}
	}

	addParagraph := func(blocks []PostBlock) {
		flushParagraph()
		paragraphs = append(paragraphs, blocks)
	}

	for _, line := range lines {
		// --- Code block handling ---
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End of code block.
				inCodeBlock = false
				codeText := codeBlockContent.String()
				if codeText != "" {
					addParagraph([]PostBlock{
						{Tag: "text", Text: formatPostCodeBlock(codeText, codeBlockLang)},
					})
				}
				codeBlockContent.Reset()
				codeBlockLang = ""
			} else {
				// Start of code block.
				inCodeBlock = true
				flushParagraph()
				codeBlockLang = strings.TrimSpace(strings.TrimPrefix(line, "```"))
			}
			continue
		}

		if inCodeBlock {
			if codeBlockContent.Len() > 0 {
				codeBlockContent.WriteByte('\n')
			}
			codeBlockContent.WriteString(line)
			continue
		}

		// --- Heading handling ---
		if match := postHeadingRegex.FindStringSubmatch(line); match != nil {
			level := len(match[1])
			headingText := strings.TrimSpace(match[2])

			// First H1/H2 becomes the post title.
			if title == "" && level <= 2 {
				title = headingText
				continue
			}

			// Other headings: add as bold text with spacing.
			flushParagraph()
			if len(paragraphs) > 0 {
				paragraphs = append(paragraphs, []PostBlock{{Tag: "text", Text: " "}})
			}
			continue
		}

		// --- Empty line (paragraph break) ---
		if strings.TrimSpace(line) == "" {
			flushParagraph()
			continue
		}

		// --- Unordered list ---
		if match := postListRegex.FindStringSubmatch(line); match != nil {
			indent := calculateIndentLevel(match[1])
			listText := strings.TrimSpace(match[2])
			prefix := strings.Repeat("  ", indent) + "• "
			addParagraph(parsePostInline(prefix + listText))
			continue
		}

		// --- Numbered list ---
		if match := postNumListRegex.FindStringSubmatch(line); match != nil {
			indent := calculateIndentLevel(match[1])
			num := match[2]
			listText := strings.TrimSpace(match[3])
			prefix := strings.Repeat("  ", indent) + num + ". "
			addParagraph(parsePostInline(prefix + listText))
			continue
		}

		// --- Blockquote ---
		if strings.HasPrefix(line, "> ") || line == ">" {
			quoteText := strings.TrimPrefix(line, "> ")
			quoteText = strings.TrimPrefix(quoteText, ">")
			blocks := parsePostInline(quoteText)
			blocks = append([]PostBlock{{Tag: "text", Text: "│ "}}, blocks...)
			addParagraph(blocks)
			continue
		}

		// --- Horizontal rule ---
		if line == "---" || line == "***" || line == "___" {
			addParagraph([]PostBlock{{Tag: "text", Text: "──────────"}})
			continue
		}

		// --- Regular text ---
		currentParagraph = append(currentParagraph, parsePostInline(line)...)
	}

	// Flush remaining.
	flushParagraph()

	// Ensure at least one paragraph.
	if len(paragraphs) == 0 {
		paragraphs = [][]PostBlock{{{Tag: "text", Text: " "}}}
	}

	return &PostContent{
		ZhCN: &PostBody{
			Title:   title,
			Content: sanitizePostBlocks(paragraphs),
		},
	}
}

// calculateIndentLevel returns the nesting level based on whitespace.
func calculateIndentLevel(indent string) int {
	spaces := 0
	for _, ch := range indent {
		if ch == '\t' {
			spaces += 4
		} else {
			spaces++
		}
	}
	return spaces / 2 // 2 spaces per level
}

// formatPostCodeBlock formats a code block for Post display.
// Since Post doesn't support fenced code blocks, we use visual separators.
func formatPostCodeBlock(code, lang string) string {
	var sb strings.Builder
	if lang != "" {
		sb.WriteString("━━━ ")
		sb.WriteString(lang)
		sb.WriteString(" ━━━\n")
	} else {
		sb.WriteString("━━━━━━━━━━\n")
	}
	sb.WriteString(code)
	sb.WriteString("\n━━━━━━━━━━")
	return sb.String()
}

// --- Post Inline Parsing ---

// parsePostInline parses inline Markdown elements for Post format.
// Handles: inline code, links, bold, italic.
// Post messages don't support rich styles, so bold/italic markers are stripped.
func parsePostInline(text string) []PostBlock {
	var blocks []PostBlock
	remaining := text

	for len(remaining) > 0 {
		// Find the next inline code span.
		idx := strings.Index(remaining, "`")
		if idx == -1 {
			blocks = append(blocks, parsePostLinks(remaining)...)
			break
		}

		// Text before the code span.
		if idx > 0 {
			blocks = append(blocks, parsePostLinks(remaining[:idx])...)
		}

		// Find closing backtick.
		closeIdx := strings.Index(remaining[idx+1:], "`")
		if closeIdx == -1 {
			// No closing backtick — treat as normal text.
			blocks = append(blocks, PostBlock{Tag: "text", Text: remaining[idx:]})
			break
		}

		// Inline code: extract content (Post doesn't support code style).
		codeText := remaining[idx+1 : idx+1+closeIdx]
		blocks = append(blocks, PostBlock{Tag: "text", Text: "「" + codeText + "」"})
		remaining = remaining[idx+1+closeIdx+1:]
	}

	return blocks
}

// parsePostLinks extracts [text](url) links and processes remaining text styles.
func parsePostLinks(text string) []PostBlock {
	var blocks []PostBlock

	matches := postLinkRegex.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return stripStyles(text)
	}

	lastEnd := 0
	for _, match := range matches {
		// Text before link.
		if match[0] > lastEnd {
			blocks = append(blocks, stripStyles(text[lastEnd:match[0]])...)
		}
		// Link.
		linkText := text[match[2]:match[3]]
		linkURL := text[match[4]:match[5]]
		blocks = append(blocks, PostBlock{
			Tag:  "text",
			Text: linkText,
			Href: linkURL,
		})
		lastEnd = match[1]
	}

	// Text after last link.
	if lastEnd < len(text) {
		blocks = append(blocks, stripStyles(text[lastEnd:])...)
	}

	return blocks
}

// stripStyles removes bold (**) and italic (*) markers from text.
// Post messages don't support rich text styles, so we keep the content only.
func stripStyles(text string) []PostBlock {
	// Remove ** markers (bold).
	result := removePairedMarkers(text, "**")
	// Remove * markers (italic) — only single asterisks, not part of **.
	result = removePairedMarkers(result, "*")
	// Remove ~~ markers (strikethrough).
	result = removePairedMarkers(result, "~~")

	if result == "" {
		return nil
	}
	return []PostBlock{{Tag: "text", Text: result}}
}

// removePairedMarkers removes paired markers (e.g., ** for bold) from text.
func removePairedMarkers(text, marker string) string {
	for {
		start := strings.Index(text, marker)
		if start == -1 {
			break
		}
		end := strings.Index(text[start+len(marker):], marker)
		if end == -1 {
			break
		}
		// Remove the markers, keep the content.
		text = text[:start] + text[start+len(marker):start+len(marker)+end] + text[start+len(marker)+end+len(marker):]
	}
	return text
}

// sanitizePostBlocks ensures all PostBlock text fields are valid for Feishu API.
// Feishu API requires text tag's text field to be non-empty when using json:"text,omitempty".
func sanitizePostBlocks(paragraphs [][]PostBlock) [][]PostBlock {
	var result [][]PostBlock
	for _, para := range paragraphs {
		var sanitized []PostBlock
		for _, block := range para {
			if block.Tag == "text" && block.Text == "" {
				block.Text = " "
			}
			sanitized = append(sanitized, block)
		}
		if len(sanitized) > 0 {
			result = append(result, sanitized)
		}
	}
	if len(result) == 0 {
		result = [][]PostBlock{{{Tag: "text", Text: " "}}}
	}
	return result
}

// --- Card Builder ---

// cardHeadingRegex matches markdown headings for title extraction.
var cardHeadingRegex = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)

// extractCardContent extracts the first heading as card title
// and returns the remaining text as card-compatible markdown.
func extractCardContent(text string) *CardContent {
	lines := strings.Split(text, "\n")

	var title string
	var result strings.Builder
	firstContent := true

	for _, line := range lines {
		// Extract first H1/H2 as card title.
		if match := cardHeadingRegex.FindStringSubmatch(line); match != nil {
			level := len(match[1])
			headingText := strings.TrimSpace(match[2])

			if title == "" && level <= 2 {
				title = headingText
				continue
			}
		}

		// All other lines pass through.
		if firstContent {
			firstContent = false
		} else {
			result.WriteByte('\n')
		}
		result.WriteString(line)
	}

	return &CardContent{
		Title:    title,
		Markdown: result.String(),
	}
}

// buildCard constructs a Feishu interactive card JSON from CardContent.
//
// Card structure (v2):
//
//	{
//	  "config": { "wide_screen_mode": true, "update_multi": true },
//	  "header": { "title": {...}, "template": "blue" },
//	  "elements": [
//	    { "tag": "markdown", "content": "..." },
//	    { "tag": "hr" },  // optional separator
//	    { "tag": "markdown", "content": "..." }
//	  ]
//	}
//
// Long content is split into multiple markdown elements with HR separators
// to avoid hitting Feishu's per-element content limit.
func buildCard(content *CardContent) map[string]interface{} {
	card := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
			"update_multi":     true,
		},
	}

	// Set card header if title was extracted.
	if content.Title != "" {
		card["header"] = map[string]interface{}{
			"title": map[string]interface{}{
				"content": content.Title,
				"tag":     "plain_text",
			},
			"template": "blue",
		}
	}

	// Build elements: split long content into chunks.
	elements := buildCardElements(content.Markdown)

	// Ensure at least one element.
	if len(elements) == 0 {
		elements = []map[string]interface{}{
			{"tag": "markdown", "content": " "},
		}
	}

	card["elements"] = elements
	return card
}

// buildCardElements splits markdown into card elements.
// If content is short, returns a single markdown element.
// If content is long, splits at natural boundaries (headings, HR) into multiple elements.
const maxCardElementLen = 3000 // Feishu per-element content limit (~4000 chars, use 3000 for safety)

func buildCardElements(markdown string) []map[string]interface{} {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return nil
	}

	// Short content: single element.
	if len(markdown) <= maxCardElementLen {
		return []map[string]interface{}{
			{"tag": "markdown", "content": markdown},
		}
	}

	// Long content: split into chunks.
	chunks := splitMarkdownChunks(markdown, maxCardElementLen)
	var elements []map[string]interface{}

	for i, chunk := range chunks {
		if i > 0 {
			elements = append(elements, map[string]interface{}{"tag": "hr"})
		}
		elements = append(elements, map[string]interface{}{
			"tag":     "markdown",
			"content": strings.TrimSpace(chunk),
		})
	}

	return elements
}

// splitMarkdownChunks splits markdown at natural boundaries (double newlines, headings).
func splitMarkdownChunks(text string, maxLen int) []string {
	var chunks []string
	var current strings.Builder

	paragraphs := strings.Split(text, "\n\n")
	for _, para := range paragraphs {
		// Check if adding this paragraph would exceed the limit.
		if current.Len() > 0 && current.Len()+len(para)+2 > maxLen {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}
