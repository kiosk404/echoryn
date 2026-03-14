// Package markdown provides Markdown rendering for terminal output.
// Supports code highlighting, tables, lists, and inline styles.
package markdown

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kiosk404/echoryn/pkg/cli/tui/bubbletea/theme"
)

// Renderer renders Markdown content to styled terminal output.
type Renderer struct {
	width       int
	theme       *theme.SemanticColors
	styles      *theme.Styles
	codeTheme   *CodeTheme
	lineNumbers bool
}

// CodeTheme defines colors for syntax highlighting.
type CodeTheme struct {
	Keyword  lipgloss.Color
	String   lipgloss.Color
	Comment  lipgloss.Color
	Number   lipgloss.Color
	Function lipgloss.Color
	Operator lipgloss.Color
	Type     lipgloss.Color
	Variable lipgloss.Color
	Default  lipgloss.Color
}

// DefaultCodeTheme returns the default code theme.
func DefaultCodeTheme() *CodeTheme {
	return &CodeTheme{
		Keyword:  lipgloss.Color("197"), // Magenta
		String:   lipgloss.Color("82"),  // Green
		Comment:  lipgloss.Color("243"), // Gray
		Number:   lipgloss.Color("214"), // Orange
		Function: lipgloss.Color("81"),  // Blue
		Operator: lipgloss.Color("208"), // Orange
		Type:     lipgloss.Color("39"),  // Cyan
		Variable: lipgloss.Color("252"), // Light gray
		Default:  lipgloss.Color("252"), // Light gray
	}
}

// NewRenderer creates a new Markdown renderer.
func NewRenderer(width int) *Renderer {
	return &Renderer{
		width:     width,
		theme:     theme.GetTheme(),
		styles:    theme.GetStyles(),
		codeTheme: DefaultCodeTheme(),
	}
}

// SetWidth sets the rendering width.
func (r *Renderer) SetWidth(width int) {
	r.width = width
}

// SetLineNumbers enables/disables line numbers in code blocks.
func (r *Renderer) SetLineNumbers(enabled bool) {
	r.lineNumbers = enabled
}

// Render renders Markdown text to styled terminal output.
func (r *Renderer) Render(text string) string {
	var result strings.Builder
	lines := strings.Split(text, "\n")

	inCodeBlock := false
	codeLang := ""
	codeLines := []string{}

	for _, line := range lines {
		// Handle code blocks
		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				inCodeBlock = true
				codeLang = strings.TrimPrefix(line, "```")
				codeLines = []string{}
				continue
			} else {
				// End code block
				result.WriteString(r.renderCodeBlock(codeLang, codeLines))
				result.WriteString("\n")
				inCodeBlock = false
				codeLang = ""
				codeLines = nil
				continue
			}
		}

		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		// Handle other elements
		rendered := r.renderLine(line)
		result.WriteString(rendered)
		result.WriteString("\n")
	}

	return result.String()
}

// renderLine renders a single line of Markdown.
func (r *Renderer) renderLine(line string) string {
	// Empty line
	if strings.TrimSpace(line) == "" {
		return ""
	}

	// Headers
	if strings.HasPrefix(line, "#### ") {
		text := strings.TrimPrefix(line, "#### ")
		return r.styles.AssistantLabel.Render("#### ") + r.styles.AssistantContent.Bold(true).Render(text)
	}
	if strings.HasPrefix(line, "### ") {
		text := strings.TrimPrefix(line, "### ")
		return r.styles.AssistantLabel.Render("### ") + r.styles.AssistantContent.Bold(true).Render(text)
	}
	if strings.HasPrefix(line, "## ") {
		text := strings.TrimPrefix(line, "## ")
		return r.styles.AssistantLabel.Render("## ") + r.styles.AssistantContent.Bold(true).Render(text)
	}
	if strings.HasPrefix(line, "# ") {
		text := strings.TrimPrefix(line, "# ")
		return r.styles.AssistantLabel.Render("# ") + r.styles.AssistantContent.Bold(true).Render(text)
	}

	// Horizontal rule
	if matched, _ := regexp.MatchString(`^ *([-*_] *){3,} *$`, line); matched {
		return lipgloss.NewStyle().Foreground(r.theme.Border.Default).Render(strings.Repeat("─", r.width))
	}

	// Unordered list
	if matched, _ := regexp.MatchString(`^([ \t]*)([-*+]) +(.*)`, line); matched {
		re := regexp.MustCompile(`^([ \t]*)([-*+]) +(.*)`)
		matches := re.FindStringSubmatch(line)
		indent := matches[1]
		marker := matches[2]
		text := r.renderInline(matches[3])
		return indent + r.styles.ToolName.Render(marker+" ") + text
	}

	// Ordered list
	if matched, _ := regexp.MatchString(`^([ \t]*)(\d+)\. +(.*)`, line); matched {
		re := regexp.MustCompile(`^([ \t]*)(\d+)\. +(.*)`)
		matches := re.FindStringSubmatch(line)
		indent := matches[1]
		num := matches[2]
		text := r.renderInline(matches[3])
		return indent + r.styles.ToolName.Render(num+". ") + text
	}

	// Table (simple detection)
	if strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") {
		return r.renderTableRow(line)
	}

	// Blockquote
	if strings.HasPrefix(line, "> ") {
		text := strings.TrimPrefix(line, "> ")
		return r.styles.SystemMessage.Render("│ ") + r.styles.SystemMessage.Render(text)
	}

	// Regular paragraph with inline formatting
	return r.renderInline(line)
}

// renderInline renders inline Markdown elements.
func (r *Renderer) renderInline(text string) string {
	// Bold **text** or __text__
	boldRe := regexp.MustCompile(`(\*\*|__)(.*?)\1`)
	text = boldRe.ReplaceAllStringFunc(text, func(m string) string {
		re := regexp.MustCompile(`(\*\*|__)(.*?)\1`)
		matches := re.FindStringSubmatch(m)
		return r.styles.AssistantContent.Bold(true).Render(matches[2])
	})

	// Italic *text* or _text_
	italicRe := regexp.MustCompile(`(\*|_)(.*?)\1`)
	text = italicRe.ReplaceAllStringFunc(text, func(m string) string {
		re := regexp.MustCompile(`(\*|_)(.*?)\1`)
		matches := re.FindStringSubmatch(m)
		return r.styles.AssistantContent.Italic(true).Render(matches[2])
	})

	// Code `text`
	codeRe := regexp.MustCompile("`([^`]+)`")
	text = codeRe.ReplaceAllStringFunc(text, func(m string) string {
		re := regexp.MustCompile("`([^`]+)`")
		matches := re.FindStringSubmatch(m)
		return r.styles.CodeBlock.Render(matches[1])
	})

	// Links [text](url)
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	text = linkRe.ReplaceAllStringFunc(text, func(m string) string {
		re := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
		matches := re.FindStringSubmatch(m)
		return lipgloss.NewStyle().
			Foreground(r.theme.Text.Link).
			Underline(true).
			Render(matches[1]) + lipgloss.NewStyle().
			Foreground(r.theme.Text.Secondary).
			Render("("+matches[2]+")")
	})

	return text
}

// renderCodeBlock renders a code block with optional syntax highlighting.
func (r *Renderer) renderCodeBlock(lang string, lines []string) string {
	var result strings.Builder

	// Header with language
	if lang != "" {
		header := r.styles.CodeHeader.Render(fmt.Sprintf("┌─ %s ─", lang))
		// Pad to width
		headerPad := r.width - lipgloss.Width(header) - 1
		if headerPad > 0 {
			header += r.styles.CodeHeader.Render(strings.Repeat("─", headerPad))
		}
		result.WriteString(header)
		result.WriteString("\n")
	} else {
		result.WriteString(r.styles.CodeHeader.Render(strings.Repeat("─", r.width)))
		result.WriteString("\n")
	}

	// Code lines
	for i, line := range lines {
		// Line numbers
		if r.lineNumbers {
			ln := r.styles.CodeLineNum.Render(fmt.Sprintf("%3d │ ", i+1))
			result.WriteString(ln)
		} else {
			result.WriteString(r.styles.CodeBlock.Render("  "))
		}

		// Syntax highlighting
		highlighted := r.highlightCode(line, lang)
		result.WriteString(r.styles.CodeContent.Render(highlighted))
		result.WriteString("\n")
	}

	// Footer
	result.WriteString(r.styles.CodeHeader.Render(strings.Repeat("─", r.width)))

	return result.String()
}

// highlightCode applies basic syntax highlighting to code.
func (r *Renderer) highlightCode(line, lang string) string {
	if lang == "" {
		return line
	}

	// Simple keyword-based highlighting
	// In a full implementation, this would use a proper lexer

	keywords := []string{}

	switch lang {
	case "go", "golang":
		keywords = []string{"package", "import", "func", "return", "if", "else", "for", "range", "switch", "case", "default", "struct", "interface", "type", "var", "const", "map", "chan", "go", "defer", "select"}
	case "javascript", "js", "typescript", "ts":
		keywords = []string{"function", "return", "if", "else", "for", "while", "switch", "case", "default", "const", "let", "var", "class", "interface", "type", "import", "export", "from", "async", "await", "try", "catch"}
	case "python", "py":
		keywords = []string{"def", "class", "return", "if", "elif", "else", "for", "while", "import", "from", "as", "with", "try", "except", "finally", "raise", "lambda", "yield", "async", "await"}
	case "java":
		keywords = []string{"public", "private", "protected", "class", "interface", "extends", "implements", "return", "if", "else", "for", "while", "switch", "case", "default", "try", "catch", "finally", "throw", "new", "static", "final", "void", "int", "long", "double", "float", "boolean", "char", "String"}
	case "bash", "sh", "shell":
		keywords = []string{"if", "then", "else", "fi", "for", "do", "done", "while", "case", "esac", "function", "return", "exit", "echo", "export", "source", "alias"}
	}

	// Highlight keywords
	result := line
	for _, kw := range keywords {
		// Match whole word
		re := regexp.MustCompile(`\b` + kw + `\b`)
		result = re.ReplaceAllStringFunc(result, func(m string) string {
			return lipgloss.NewStyle().Foreground(r.codeTheme.Keyword).Render(m)
		})
	}

	// Highlight strings (double quotes)
	strRe := regexp.MustCompile(`"([^"\\]|\\.)*"`)
	result = strRe.ReplaceAllStringFunc(result, func(m string) string {
		return lipgloss.NewStyle().Foreground(r.codeTheme.String).Render(m)
	})

	// Highlight strings (single quotes)
	strRe2 := regexp.MustCompile(`'([^'\\]|\\.)*'`)
	result = strRe2.ReplaceAllStringFunc(result, func(m string) string {
		return lipgloss.NewStyle().Foreground(r.codeTheme.String).Render(m)
	})

	// Highlight comments
	if lang == "go" || lang == "golang" || lang == "javascript" || lang == "js" || lang == "typescript" || lang == "ts" || lang == "java" {
		// Single line comment
		if idx := strings.Index(result, "//"); idx >= 0 {
			before := result[:idx]
			after := result[idx:]
			result = before + lipgloss.NewStyle().Foreground(r.codeTheme.Comment).Render(after)
		}
	} else if lang == "python" || lang == "py" || lang == "bash" || lang == "sh" {
		if idx := strings.Index(result, "#"); idx >= 0 {
			before := result[:idx]
			after := result[idx:]
			result = before + lipgloss.NewStyle().Foreground(r.codeTheme.Comment).Render(after)
		}
	}

	// Highlight numbers
	numRe := regexp.MustCompile(`\b(\d+\.?\d*)\b`)
	result = numRe.ReplaceAllStringFunc(result, func(m string) string {
		return lipgloss.NewStyle().Foreground(r.codeTheme.Number).Render(m)
	})

	return result
}

// renderTableRow renders a table row.
func (r *Renderer) renderTableRow(line string) string {
	// Check if it's a separator row
	sepRe := regexp.MustCompile(`^\s*\|?[\s:|-]+\|\s*$`)
	if sepRe.MatchString(line) {
		return lipgloss.NewStyle().Foreground(r.theme.Border.Default).Render(strings.Repeat("─", r.width))
	}

	// Parse cells
	cells := strings.Split(line, "|")
	var styledCells []string
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		styledCells = append(styledCells, r.renderInline(cell))
	}

	if len(styledCells) == 0 {
		return ""
	}

	// Render with borders
	result := "│ "
	result += strings.Join(styledCells, " │ ")
	result += " │"

	return result
}

// =============================================================================
// Convenience functions
// =============================================================================

// Render renders Markdown using default settings.
func Render(text string, width int) string {
	r := NewRenderer(width)
	return r.Render(text)
}

// RenderCode renders a code block with syntax highlighting.
func RenderCode(lang string, code string, width int, lineNumbers bool) string {
	r := NewRenderer(width)
	r.SetLineNumbers(lineNumbers)
	return r.renderCodeBlock(lang, strings.Split(code, "\n"))
}
