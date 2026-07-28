package fileops

import (
	"errors"
	"strings"
)

// Range denotes a byte offset range [Start, End) in source content.
type Range struct {
	Start int
	End   int
}

// ErrNoMatch is returned by FindBestMatch / FindAllExact when the needle
// cannot be located in the content under any supported matching strategy.
var ErrNoMatch = errors.New("no match found")

// FindBestMatch locates `needle` in `content` using a cascading ladder of
// matching strategies, from strictest to most lenient:
//
//  1. Exact substring match.
//  2. Per-line whitespace-trimmed match.
//  3. Line-ending normalized match (\r\n / \r -> \n).
//  4. Indent-normalized match (strip leading whitespace from each line).
//
// Returns (start, length, nil) on match, where length is the byte span in
// the *original* content that should be replaced. Returns ErrNoMatch if no
// strategy succeeds.
func FindBestMatch(content, needle string) (int, int, error) {
	if needle == "" {
		return -1, 0, errors.New("empty needle")
	}
	if i := strings.Index(content, needle); i >= 0 {
		return i, len(needle), nil
	}
	if i, l, ok := findByLineTransform(content, needle, strings.TrimSpace); ok {
		return i, l, nil
	}
	if i, l, ok := findByContentTransform(content, needle, normalizeLineEndings); ok {
		return i, l, nil
	}
	if i, l, ok := findByLineTransform(content, needle, stripLeadingWhitespace); ok {
		return i, l, nil
	}
	return -1, 0, ErrNoMatch
}

// FindAllExact returns every exact (byte-for-byte) occurrence of `needle`
// in `content`. Does not apply fuzzy strategies -- use FindBestMatch for
// fuzzy matching.
func FindAllExact(content, needle string) ([]Range, error) {
	if needle == "" {
		return nil, errors.New("empty needle")
	}
	var out []Range
	base := 0
	s := content
	for {
		i := strings.Index(s, needle)
		if i < 0 {
			break
		}
		out = append(out, Range{Start: base + i, End: base + i + len(needle)})
		step := i + len(needle)
		base += step
		s = s[step:]
	}
	if len(out) == 0 {
		return nil, ErrNoMatch
	}
	return out, nil
}

// --- strategy helpers ---

func stripLeadingWhitespace(s string) string {
	return strings.TrimLeft(s, " \t")
}

func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// findByLineTransform applies `fn` to every line of both content and needle,
// matches on the normalized form, and maps back to the original byte
// offset/length so callers can splice the replacement into the original text.
func findByLineTransform(content, needle string, fn func(string) string) (int, int, bool) {
	contentLines := strings.Split(content, "\n")
	normalize := func(s string) []string {
		ls := strings.Split(s, "\n")
		out := make([]string, len(ls))
		for i, l := range ls {
			out[i] = fn(l)
		}
		return out
	}
	cN := normalize(content)
	nN := normalize(needle)
	if len(nN) == 0 {
		return 0, 0, false
	}
	for i := 0; i <= len(cN)-len(nN); i++ {
		matched := true
		for j := range nN {
			if cN[i+j] != nN[j] {
				matched = false
				break
			}
		}
		if matched {
			// Compute byte offset of line i in the original content.
			offset := 0
			for k := 0; k < i; k++ {
				offset += len(contentLines[k]) + 1 // +1 for '\n'
			}
			// Compute matched byte length in original content.
			length := 0
			for k := 0; k < len(nN); k++ {
				length += len(contentLines[i+k])
				if k < len(nN)-1 {
					length++ // '\n' between needle lines
				}
			}
			return offset, length, true
		}
	}
	return 0, 0, false
}

// findByContentTransform applies `fn` to the whole content and whole needle,
// then re-maps the normalized offset back to the original text. Currently
// used for line-ending normalization where `\r\n` collapses to `\n`.
func findByContentTransform(content, needle string, fn func(string) string) (int, int, bool) {
	nc := fn(content)
	nn := fn(needle)
	i := strings.Index(nc, nn)
	if i < 0 {
		return 0, 0, false
	}
	// Re-walk the original content tracking normalized position, so we can
	// identify the real byte offsets that correspond to the normalized match.
	orig, norm := 0, 0
	for orig < len(content) && norm < i {
		if content[orig] == '\r' && orig+1 < len(content) && content[orig+1] == '\n' {
			orig += 2
			norm++
		} else {
			orig++
			norm++
		}
	}
	start := orig
	for orig < len(content) && norm < i+len(nn) {
		if content[orig] == '\r' && orig+1 < len(content) && content[orig+1] == '\n' {
			orig += 2
			norm++
		} else {
			orig++
			norm++
		}
	}
	return start, orig - start, true
}
