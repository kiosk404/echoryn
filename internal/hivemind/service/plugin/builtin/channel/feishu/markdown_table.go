package feishu

import (
	"regexp"
	"strings"
)

// TableTransformer converts standard Markdown tables into Feishu-compatible formats.
//
// Feishu's Card markdown element natively supports table rendering (cardVersion >= 2),
// but Post messages do NOT. This transformer handles the difference:
//
//   - Card mode: Tables are kept as-is (native support), but spacing is optimized.
//   - Post mode: Tables are converted to bullet-point lists for readability.
//
// Algorithm for Post mode (bullets conversion):
//  1. Detect table blocks: header row + separator row + data rows.
//  2. Parse header columns from the header row.
//  3. For each data row, pair header:value and format as "• header: value".
//  4. Separate records with blank lines.
//
// Reference: openclaw-lark's runtime.channel.text.convertMarkdownTables(text, 'bullets')
type TableTransformer struct{}

// tableBlockRegex matches a complete Markdown table block:
// header | separator | one or more data rows.
var tableBlockRegex = regexp.MustCompile(
	`(?m)((?:^[ \t]*\|.+\|[ \t]*\n)` + // header row
		`(?:^[ \t]*\|[-:\| \t]+\|[ \t]*\n)` + // separator row
		`(?:^[ \t]*\|.+\|[ \t]*(?:\n|$))+)`, // data rows
)

// tableSeparatorRegex matches the separator row (e.g., |---|---|).
var tableSeparatorRegex = regexp.MustCompile(`^[ \t]*\|[-:\| \t]+\|[ \t]*$`)

func (t *TableTransformer) Transform(text string, mode RenderMode) string {
	switch mode {
	case RenderModeCard:
		return t.optimizeCardTables(text)
	case RenderModePost:
		return t.convertToBullets(text)
	default:
		return text
	}
}

// optimizeCardTables adds spacing around tables for better card rendering.
// Reference: openclaw-lark's optimizeMarkdownStyle steps 4a-4e.
func (t *TableTransformer) optimizeCardTables(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	inTable := false

	for i, line := range lines {
		isTableLine := isTableRow(line)

		if isTableLine && !inTable {
			// Entering table: ensure blank line before
			if i > 0 && len(result) > 0 && strings.TrimSpace(result[len(result)-1]) != "" {
				result = append(result, "")
			}
			inTable = true
		}

		if !isTableLine && inTable {
			// Leaving table: ensure blank line after
			if strings.TrimSpace(line) != "" {
				result = append(result, "")
			}
			inTable = false
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// convertToBullets converts Markdown tables to bullet-point lists.
// Each data row becomes a group of "• header: value" items.
func (t *TableTransformer) convertToBullets(text string) string {
	return tableBlockRegex.ReplaceAllStringFunc(text, func(tableBlock string) string {
		return t.tableBlockToBullets(tableBlock)
	})
}

// tableBlockToBullets converts a single table block to bullet format.
func (t *TableTransformer) tableBlockToBullets(block string) string {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	if len(lines) < 3 {
		return block // Not a valid table (need header + separator + at least 1 data row)
	}

	// Parse header columns
	headers := parseTableRow(lines[0])
	if len(headers) == 0 {
		return block
	}

	// Skip separator row (lines[1])
	// Process data rows
	var sb strings.Builder
	for i, line := range lines[2:] {
		if tableSeparatorRegex.MatchString(line) {
			continue // Skip extra separator rows
		}

		cells := parseTableRow(line)
		if len(cells) == 0 {
			continue
		}

		if i > 0 {
			sb.WriteByte('\n') // Blank line between records
		}

		for j, header := range headers {
			value := ""
			if j < len(cells) {
				value = cells[j]
			}
			if value == "" {
				value = "-"
			}
			sb.WriteString("• ")
			sb.WriteString(header)
			sb.WriteString(": ")
			sb.WriteString(value)
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

// parseTableRow extracts cell values from a Markdown table row.
// Input: "| foo | bar | baz |"
// Output: ["foo", "bar", "baz"]
func parseTableRow(row string) []string {
	row = strings.TrimSpace(row)
	if !strings.HasPrefix(row, "|") || !strings.HasSuffix(row, "|") {
		return nil
	}

	// Remove leading and trailing pipes
	row = row[1 : len(row)-1]

	parts := strings.Split(row, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}

// isTableRow checks if a line looks like a Markdown table row.
func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")
}
