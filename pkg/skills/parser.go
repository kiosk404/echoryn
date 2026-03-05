package skills

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parser handles parsing of SKILL.md files.
type Parser struct{}

// NewParser creates a new SKILL.md parser.
func NewParser() *Parser {
	return &Parser{}
}

// ParseFile parses a SKILL.md file from the given path.
// Returns the frontmatter, markdown body, and any error.
func (p *Parser) ParseFile(path string) (*Frontmatter, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file: %w", err)
	}
	return p.Parse(data)
}

// Parse parses SKILL.md content bytes and extracts frontmatter and body.
func (p *Parser) Parse(data []byte) (*Frontmatter, string, error) {
	fmBytes, body, err := p.splitFrontmatter(data)
	if err != nil {
		return nil, "", err
	}

	var fm Frontmatter
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return nil, "", &SkillError{
			Message: "failed to parse YAML frontmatter",
			Err:     err,
		}
	}

	if err := fm.Validate(); err != nil {
		return nil, "", err
	}

	return &fm, body, nil
}

// ParseMetadataOnly extracts only the frontmatter without loading the full body.
// This is more efficient for initial skill discovery.
func (p *Parser) ParseMetadataOnly(path string) (*Frontmatter, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var frontmatterLines []string
	inFrontmatter := false
	lineCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break // end of frontmatter
		}

		if inFrontmatter {
			frontmatterLines = append(frontmatterLines, line)
		}

		// Safety limit — frontmatter shouldn't exceed 100 lines.
		if lineCount > 100 {
			return nil, &SkillError{Message: "frontmatter too long or missing closing delimiter"}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	if len(frontmatterLines) == 0 {
		return nil, ErrInvalidFrontmatter
	}

	var fm Frontmatter
	yamlContent := strings.Join(frontmatterLines, "\n")
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, &SkillError{
			Message: "failed to parse YAML frontmatter",
			Err:     err,
		}
	}

	if err := fm.Validate(); err != nil {
		return nil, err
	}

	return &fm, nil
}

// splitFrontmatter separates YAML frontmatter from markdown body.
func (p *Parser) splitFrontmatter(data []byte) (frontmatter []byte, body string, err error) {
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("---")) {
		return nil, "", ErrInvalidFrontmatter
	}

	lines := bytes.Split(data, []byte("\n"))
	var fmStart, fmEnd int
	foundStart := false

	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.Equal(trimmed, []byte("---")) {
			if !foundStart {
				fmStart = i
				foundStart = true
			} else {
				fmEnd = i
				break
			}
		}
	}

	if fmEnd == 0 {
		return nil, "", &SkillError{Message: "missing closing frontmatter delimiter"}
	}

	// Extract frontmatter (excluding delimiters).
	var fmLines [][]byte
	for i := fmStart + 1; i < fmEnd; i++ {
		fmLines = append(fmLines, lines[i])
	}
	frontmatter = bytes.Join(fmLines, []byte("\n"))

	// Extract body (everything after closing delimiter).
	var bodyLines [][]byte
	for i := fmEnd + 1; i < len(lines); i++ {
		bodyLines = append(bodyLines, lines[i])
	}
	body = strings.TrimSpace(string(bytes.Join(bodyLines, []byte("\n"))))

	return frontmatter, body, nil
}

// ExtractSection extracts a specific markdown section by heading.
// Heading matching is case-insensitive.
func (p *Parser) ExtractSection(body, heading string) string {
	lines := strings.Split(body, "\n")
	var result []string
	inSection := false
	sectionLevel := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			level := countPrefix(line, '#')
			headingText := strings.TrimSpace(strings.TrimLeft(line, "#"))

			if strings.EqualFold(headingText, heading) {
				inSection = true
				sectionLevel = level
				result = append(result, line)
				continue
			}

			if inSection && level <= sectionLevel {
				break
			}
		}

		if inSection {
			result = append(result, line)
		}
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

// ExtractTOC extracts all markdown headings and returns a formatted table of contents.
// Each heading is indented based on its level (H1 = no indent, H2 = 2 spaces, etc.).
func (p *Parser) ExtractTOC(body string) string {
	lines := strings.Split(body, "\n")
	var toc []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			level := countPrefix(trimmed, '#')
			headingText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))

			if headingText == "" {
				continue
			}

			indent := strings.Repeat(" ", (level-1)*2)
			toc = append(toc, fmt.Sprintf("%s%s %s", indent, strings.Repeat("#", level), headingText))
		}
	}

	return strings.Join(toc, "\n")
}

// countPrefix counts how many times a character appears at the start of a string.
func countPrefix(s string, char rune) int {
	count := 0
	for _, c := range s {
		if c == char {
			count++
		} else {
			break
		}
	}
	return count
}
