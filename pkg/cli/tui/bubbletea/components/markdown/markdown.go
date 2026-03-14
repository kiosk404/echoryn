// Package markdown provides markdown rendering for terminal output.
package markdown

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/theme"
)

// Renderer renders markdown content for terminal display.
type Renderer struct {
	width int
	theme *theme.SemanticColors
}

// NewRenderer creates a new markdown renderer.
func NewRenderer(width int) *Renderer {
	return &Renderer{
		width: width,
		theme: theme.GetTheme(),
	}
}

// Render renders markdown content to styled terminal output.
func (r *Renderer) Render(content string) string {
	styles := theme.GetStyles()

	// Process code blocks first
	content = r.renderCodeBlocks(content)

	// Process inline elements
	content = r.renderInlineCode(content)
	content = r.renderBold(content)
	content = r.renderItalic(content)
	content = r.renderLinks(content)

	// Word wrap
	content = r.wrapText(content, r.width-3)

	return styles.AssistantContent.Render(content)
}

// renderCodeBlocks renders fenced code blocks.
func (r *Renderer) renderCodeBlocks(content string) string {
	re := regexp.MustCompile("```(\\w*)\n([\\s\\S]*?)\n```")
	return re.ReplaceAllStringFunc(content, func(match string) string {
		parts := re.FindStringSubmatch(match)
		lang := parts[1]
		code := parts[2]
		return RenderCode(lang, code, r.width, true)
	})
}

// renderInlineCode renders inline code.
func (r *Renderer) renderInlineCode(content string) string {
	re := regexp.MustCompile("`([^`]+)`")
	styles := theme.GetStyles()
	return re.ReplaceAllStringFunc(content, func(match string) string {
		code := re.FindStringSubmatch(match)[1]
		return styles.ToolArgs.Render(code)
	})
}

// renderBold renders bold text.
func (r *Renderer) renderBold(content string) string {
	re := regexp.MustCompile("\\*\\*([^*]+)\\*\\*|__([^_]+)__")
	return re.ReplaceAllStringFunc(content, func(match string) string {
		text := strings.Trim(match, "*_")
		return lipgloss.NewStyle().Bold(true).Render(text)
	})
}

// renderItalic renders italic text.
func (r *Renderer) renderItalic(content string) string {
	re := regexp.MustCompile("\\*([^*]+)\\*|_([^_]+)_")
	return re.ReplaceAllStringFunc(content, func(match string) string {
		text := strings.Trim(match, "*_")
		return lipgloss.NewStyle().Italic(true).Render(text)
	})
}

// renderLinks renders links.
func (r *Renderer) renderLinks(content string) string {
	re := regexp.MustCompile("\\[([^]]+)\\]\\(([^)]+)\\)")
	styles := theme.GetStyles()
	return re.ReplaceAllStringFunc(content, func(match string) string {
		parts := re.FindStringSubmatch(match)
		text := parts[1]
		url := parts[2]
		return styles.UserPrompt.Render(text) + " " + styles.HeaderInfo.Render("("+url+")")
	})
}

// wrapText wraps text to the specified width.
func (r *Renderer) wrapText(content string, width int) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		if len(line) == 0 {
			result = append(result, "")
			continue
		}

		// Simple word wrap
		if len(line) > width {
			wrapped := r.wrapLine(line, width)
			result = append(result, wrapped...)
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// wrapLine wraps a single line.
func (r *Renderer) wrapLine(line string, width int) []string {
	var result []string
	words := strings.Fields(line)

	var current strings.Builder
	for _, word := range words {
		if current.Len()+len(word)+1 > width {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(word)
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// RenderCode renders a code block with syntax highlighting.
func RenderCode(language, code string, width int, showBorder bool) string {
	styles := theme.GetStyles()
	_ = theme.GetTheme() // Used in highlightCode

	var lines []string

	// Header
	if language != "" {
		header := styles.CodeHeader.Render(fmt.Sprintf(" %s ", language))
		if showBorder {
			topBorder := "┌" + strings.Repeat("─", min(width-2, 60)) + "┐"
			lines = append(lines, topBorder)
			contentLine := "│ " + header + strings.Repeat(" ", min(width-len(header)-4, 56)) + "│"
			lines = append(lines, contentLine)
			midBorder := "├" + strings.Repeat("─", min(width-2, 60)) + "┤"
			lines = append(lines, midBorder)
		} else {
			lines = append(lines, header)
		}
	}

	// Code content
	codeLines := strings.Split(code, "\n")
	lineNum := 1

	for _, codeLine := range codeLines {
		var rendered string

		// Line number
		lineNumStr := fmt.Sprintf("%3d ", lineNum)
		rendered = styles.CodeLineNum.Render(lineNumStr)

		// Code (with basic syntax highlighting)
		rendered += highlightCode(codeLine, language)

		if showBorder {
			lines = append(lines, "│ "+rendered)
		} else {
			lines = append(lines, rendered)
		}

		lineNum++
	}

	// Footer
	if showBorder {
		botBorder := "└" + strings.Repeat("─", min(width-2, 60)) + "┘"
		lines = append(lines, botBorder)
	}

	return strings.Join(lines, "\n")
}

// highlightCode applies basic syntax highlighting.
func highlightCode(code, language string) string {
	styles := theme.GetStyles()
	t := theme.GetTheme()

	// Keywords
	keywords := getKeywords(language)
	for _, kw := range keywords {
		code = strings.ReplaceAll(code, kw, lipgloss.NewStyle().
			Foreground(t.Text.Accent).
			Render(kw))
	}

	// Strings (simple pattern)
	stringPattern := regexp.MustCompile(`"([^"]*)"|'([^']*)'`)
	code = stringPattern.ReplaceAllStringFunc(code, func(match string) string {
		return lipgloss.NewStyle().Foreground(t.Status.Success).Render(match)
	})

	// Comments
	commentPattern := regexp.MustCompile(`(//.*$|#.*$)`)
	code = commentPattern.ReplaceAllStringFunc(code, func(match string) string {
		return styles.SystemMessage.Render(match)
	})

	return code
}

// getKeywords returns keywords for syntax highlighting.
func getKeywords(language string) []string {
	commonKeywords := []string{
		"func", "return", "if", "else", "for", "range", "switch", "case",
		"break", "continue", "goto", "defer", "go", "select", "chan", "const",
		"struct", "interface", "type", "var", "package", "import", "map", "slice",
		"make", "new", "len", "cap", "append", "copy", "delete", "panic",
		"recover", "error", "nil", "true", "false", "iota", "main",
	}

	switch strings.ToLower(language) {
	case "go", "golang":
		return commonKeywords
	case "python", "py":
		return []string{
			"def", "class", "return", "if", "else", "elif", "for", "while",
			"break", "continue", "pass", "import", "from", "as", "with",
			"try", "except", "finally", "raise", "lambda", "yield", "global",
			"True", "False", "None", "and", "or", "not", "in", "is",
		}
	case "javascript", "js", "typescript", "ts":
		return []string{
			"function", "return", "if", "else", "for", "while", "do", "switch",
			"break", "continue", "try", "catch", "finally", "throw", "new",
			"const", "let", "var", "class", "extends", "import", "export",
			"from", "as", "async", "await", "true", "false", "null", "undefined",
		}
	default:
		return commonKeywords
	}
}
