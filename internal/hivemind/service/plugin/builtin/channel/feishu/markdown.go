package feishu

import (
	"regexp"
	"strings"
)

// PostContent represents the Feishu post message content structure.
type PostContent struct {
	ZhCN *PostBody `json:"zh_cn,omitempty"`
}

// PostBody represents the body of a Feishu post message.
type PostBody struct {
	Title   string        `json:"title,omitempty"`
	Content [][]PostBlock `json:"content"`
}

// PostBlock represents a single block in a Feishu post message.
type PostBlock struct {
	Tag    string `json:"tag,omitempty"`
	Text   string `json:"text,omitempty"`
	Href   string `json:"href,omitempty"`
	UserID string `json:"user_id,omitempty"`
	ImgKey string `json:"img_key,omitempty"`
	Style  string `json:"style,omitempty"`
}

// MarkdownToPost converts Markdown text to Feishu post format.
// It extracts the first heading as the title and converts the rest to paragraphs.
func MarkdownToPost(markdown string) *PostContent {
	lines := strings.Split(markdown, "\n")

	var title string
	var paragraphs [][]PostBlock
	var currentParagraph []PostBlock
	var inCodeBlock bool
	var codeBlockContent strings.Builder
	var codeBlockLang string

	for _, line := range lines {
		// Handle code blocks
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End of code block
				inCodeBlock = false
				codeText := codeBlockContent.String()
				if codeText != "" {
					// Add code block as a separate paragraph
					paragraphs = append(paragraphs, []PostBlock{
						{Tag: "text", Text: formatCodeBlock(codeText, codeBlockLang)},
					})
				}
				codeBlockContent.Reset()
				codeBlockLang = ""
			} else {
				// Start of code block
				inCodeBlock = true
				// Flush current paragraph
				if len(currentParagraph) > 0 {
					paragraphs = append(paragraphs, currentParagraph)
					currentParagraph = nil
				}
				// Extract language
				codeBlockLang = strings.TrimPrefix(line, "```")
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

		// Handle headings
		if headingMatch := headingRegex.FindStringSubmatch(line); headingMatch != nil {
			level := len(headingMatch[1])
			headingText := strings.TrimSpace(headingMatch[2])

			// First heading becomes the title
			if title == "" && level <= 2 {
				title = headingText
				continue
			}

			// Other headings become styled text
			if len(currentParagraph) > 0 {
				paragraphs = append(paragraphs, currentParagraph)
				currentParagraph = nil
			}
			paragraphs = append(paragraphs, []PostBlock{
				{Tag: "text", Text: headingText, Style: "bold"},
			})
			continue
		}

		// Handle empty lines (paragraph break)
		if strings.TrimSpace(line) == "" {
			if len(currentParagraph) > 0 {
				paragraphs = append(paragraphs, currentParagraph)
				currentParagraph = nil
			}
			continue
		}

		// Handle list items
		if listMatch := listRegex.FindStringSubmatch(line); listMatch != nil {
			listText := strings.TrimSpace(listMatch[2])
			blocks := parseInlineMarkdown("• " + listText)
			if len(currentParagraph) > 0 {
				paragraphs = append(paragraphs, currentParagraph)
				currentParagraph = nil
			}
			paragraphs = append(paragraphs, blocks)
			continue
		}

		// Handle numbered list items
		if numListMatch := numListRegex.FindStringSubmatch(line); numListMatch != nil {
			num := numListMatch[1]
			listText := strings.TrimSpace(numListMatch[2])
			blocks := parseInlineMarkdown(num + ". " + listText)
			if len(currentParagraph) > 0 {
				paragraphs = append(paragraphs, currentParagraph)
				currentParagraph = nil
			}
			paragraphs = append(paragraphs, blocks)
			continue
		}

		// Handle blockquotes
		if strings.HasPrefix(line, "> ") {
			quoteText := strings.TrimPrefix(line, "> ")
			blocks := parseInlineMarkdown(quoteText)
			// Add quote indicator
			blocks = append([]PostBlock{{Tag: "text", Text: "│ "}}, blocks...)
			if len(currentParagraph) > 0 {
				paragraphs = append(paragraphs, currentParagraph)
				currentParagraph = nil
			}
			paragraphs = append(paragraphs, blocks)
			continue
		}

		// Handle horizontal rules
		if line == "---" || line == "***" || line == "___" {
			if len(currentParagraph) > 0 {
				paragraphs = append(paragraphs, currentParagraph)
				currentParagraph = nil
			}
			paragraphs = append(paragraphs, []PostBlock{
				{Tag: "text", Text: "──────────"},
			})
			continue
		}

		// Regular paragraph text
		blocks := parseInlineMarkdown(line)
		currentParagraph = append(currentParagraph, blocks...)
	}

	// Flush remaining paragraph
	if len(currentParagraph) > 0 {
		paragraphs = append(paragraphs, currentParagraph)
	}

	// Ensure at least one paragraph
	if len(paragraphs) == 0 {
		paragraphs = [][]PostBlock{{{Tag: "text", Text: ""}}}
	}

	return &PostContent{
		ZhCN: &PostBody{
			Title:   title,
			Content: paragraphs,
		},
	}
}

// parseInlineMarkdown parses inline Markdown elements (bold, italic, code, links).
func parseInlineMarkdown(text string) []PostBlock {
	var blocks []PostBlock
	remaining := text

	for len(remaining) > 0 {
		// Try to match inline code first
		if idx := strings.Index(remaining, "`"); idx != -1 {
			// Text before code
			if idx > 0 {
				blocks = append(blocks, parseInlineElements(remaining[:idx])...)
			}

			// Find closing backtick
			closeIdx := strings.Index(remaining[idx+1:], "`")
			if closeIdx != -1 {
				codeText := remaining[idx+1 : idx+1+closeIdx]
				blocks = append(blocks, PostBlock{
					Tag:  "text",
					Text: codeText,
					// Note: Feishu doesn't support code style in post,
					// but we can use a different appearance
				})
				remaining = remaining[idx+1+closeIdx+1:]
				continue
			}
			// No closing backtick, treat as normal text
			blocks = append(blocks, PostBlock{Tag: "text", Text: remaining[idx:]})
			break
		}

		// No more special elements
		blocks = append(blocks, parseInlineElements(remaining)...)
		break
	}

	return blocks
}

// parseInlineElements parses bold, italic, and links in text.
func parseInlineElements(text string) []PostBlock {
	var blocks []PostBlock

	// Process links [text](url)
	linkResult := linkRegex.FindAllStringSubmatchIndex(text, -1)
	if len(linkResult) > 0 {
		lastEnd := 0
		for _, match := range linkResult {
			// Text before link
			if match[0] > lastEnd {
				blocks = append(blocks, parseTextStyles(text[lastEnd:match[0]])...)
			}
			// Link itself
			linkText := text[match[2]:match[3]]
			linkURL := text[match[4]:match[5]]
			blocks = append(blocks, PostBlock{
				Tag:  "a",
				Text: linkText,
				Href: linkURL,
			})
			lastEnd = match[1]
		}
		// Text after last link
		if lastEnd < len(text) {
			blocks = append(blocks, parseTextStyles(text[lastEnd:])...)
		}
		return blocks
	}

	// No links, process text styles
	return parseTextStyles(text)
}

// parseTextStyles parses bold and italic in text.
func parseTextStyles(text string) []PostBlock {
	// For simplicity, convert **bold** and *italic* to text
	// Feishu post doesn't have native bold/italic in text blocks
	// We'll strip the markers for now
	result := text

	// Strip bold markers
	result = boldRegex.ReplaceAllString(result, "$1")

	// Strip italic markers (single *)
	result = italicRegex.ReplaceAllString(result, "$1")

	return []PostBlock{{Tag: "text", Text: result}}
}

// formatCodeBlock formats code block content for display.
func formatCodeBlock(code, lang string) string {
	var sb strings.Builder

	// Add language hint if available
	if lang != "" {
		sb.WriteString("━━━ ")
		sb.WriteString(lang)
		sb.WriteString(" ━━━\n")
	}

	// Add code content with monospace-friendly formatting
	sb.WriteString(code)

	return sb.String()
}

// Regular expressions for Markdown parsing
var (
	headingRegex = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	listRegex    = regexp.MustCompile(`^(\s*)[-*+]\s+(.+)$`)
	numListRegex = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.+)$`)
	linkRegex    = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	boldRegex    = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRegex  = regexp.MustCompile(`\*([^*]+)\*`)
)
