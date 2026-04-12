package webfetch

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	readability "github.com/go-shiori/go-readability"
	"golang.org/x/net/html"
)

// extractMode controls the output format.
type extractMode string

const (
	extractMarkdown extractMode = "markdown"
	extractText     extractMode = "text"
)

const (
	// readabilityMaxHTMLChars caps the HTML size before DOM parsing to prevent OOM.
	readabilityMaxHTMLChars = 1_000_000
	// readabilityMaxNestingDepth caps the estimated HTML nesting depth.
	readabilityMaxNestingDepth = 3_000
)

// ---------- DOM-level Readability Extraction ----------

// extractReadableContent uses go-readability (port of Mozilla Readability)
// for DOM-level main-content extraction, matching OpenClaw's approach.
//
// Flow: sanitizeHTML (remove hidden elements) → go-readability parse → convert.
// Falls back to regex-based htmlToMarkdown if DOM parsing fails.
func extractReadableContent(rawHTML, pageURL string, mode extractMode) (text, title string, err error) {
	// 1. Pre-sanitize: remove hidden elements via DOM walk.
	cleaned := sanitizeHTML(rawHTML)

	// 2. Guard: skip DOM parsing on oversized or deeply-nested HTML.
	if len(cleaned) > readabilityMaxHTMLChars ||
		exceedsEstimatedHTMLNestingDepth(cleaned, readabilityMaxNestingDepth) {
		md, docTitle := htmlToMarkdown(cleaned)
		if mode == extractText {
			return stripInvisibleUnicode(markdownToText(md)), docTitle, nil
		}
		return stripInvisibleUnicode(md), docTitle, nil
	}

	// 3. Parse with go-readability.
	parsedURL, _ := url.Parse(pageURL)
	article, err := readability.FromReader(strings.NewReader(cleaned), parsedURL)
	if err != nil || strings.TrimSpace(article.Content) == "" {
		// Fallback to regex-based conversion.
		md, docTitle := htmlToMarkdown(cleaned)
		if mode == extractText {
			t := stripInvisibleUnicode(markdownToText(md))
			if t == "" {
				t = stripInvisibleUnicode(normalizeWhitespace(stripTags(cleaned)))
			}
			return t, docTitle, nil
		}
		t := stripInvisibleUnicode(md)
		if t == "" {
			t = stripInvisibleUnicode(normalizeWhitespace(stripTags(cleaned)))
		}
		return t, docTitle, nil
	}

	// 4. Convert readability output.
	docTitle := article.Title
	if mode == extractText {
		text = stripInvisibleUnicode(normalizeWhitespace(article.TextContent))
		if text == "" {
			md, _ := htmlToMarkdown(article.Content)
			text = stripInvisibleUnicode(markdownToText(md))
		}
	} else {
		md, _ := htmlToMarkdown(article.Content)
		text = stripInvisibleUnicode(md)
	}

	return text, docTitle, nil
}

// ---------- HTML Nesting Depth Guard ----------

// exceedsEstimatedHTMLNestingDepth is a cheap heuristic to skip DOM parsing
// on pathological HTML (deep nesting → stack/memory blowups).
// Mirrors OpenClaw's exceedsEstimatedHtmlNestingDepth.
func exceedsEstimatedHTMLNestingDepth(htmlStr string, maxDepth int) bool {
	voidTags := map[string]bool{
		"area": true, "base": true, "br": true, "col": true,
		"embed": true, "hr": true, "img": true, "input": true,
		"link": true, "meta": true, "param": true, "source": true,
		"track": true, "wbr": true,
	}

	depth := 0
	data := []byte(htmlStr)
	length := len(data)

	for i := 0; i < length; i++ {
		if data[i] != '<' {
			continue
		}
		next := byte(0)
		if i+1 < length {
			next = data[i+1]
		}
		if next == '!' || next == '?' {
			continue
		}

		j := i + 1
		closing := false
		if j < length && data[j] == '/' {
			closing = true
			j++
		}

		// Skip whitespace.
		for j < length && data[j] <= 32 {
			j++
		}

		// Read tag name.
		nameStart := j
		for j < length {
			c := data[j]
			isName := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
				(c >= '0' && c <= '9') || c == ':' || c == '-'
			if !isName {
				break
			}
			j++
		}

		tagName := strings.ToLower(string(data[nameStart:j]))
		if tagName == "" {
			continue
		}

		if closing {
			if depth > 0 {
				depth--
			}
			continue
		}
		if voidTags[tagName] {
			continue
		}

		// Best-effort self-closing detection: scan short window for "/>".
		selfClosing := false
		limit := j + 200
		if limit > length {
			limit = length
		}
		for k := j; k < limit; k++ {
			if data[k] == '>' {
				if k > 0 && data[k-1] == '/' {
					selfClosing = true
				}
				break
			}
		}
		if selfClosing {
			continue
		}

		depth++
		if depth > maxDepth {
			return true
		}
	}
	return false
}

// ---------- DOM-level HTML Sanitization ----------

// sanitizeHTML removes hidden elements from HTML using real DOM parsing.
// This matches OpenClaw's sanitizeHtml() which uses linkedom + DOM walk.
func sanitizeHTML(rawHTML string) string {
	// 1. Strip HTML comments first (cheap string op).
	rawHTML = reComment.ReplaceAllString(rawHTML, "")

	// 2. Parse into DOM.
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		// If DOM parse fails, fall back to regex sanitization.
		return removeHiddenElementsRegex(rawHTML)
	}

	// 3. Collect elements to remove (bottom-up to avoid re-walking removed subtrees).
	var toRemove []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && shouldRemoveElement(n) {
			toRemove = append(toRemove, n)
		}
	}
	walk(doc)

	// 4. Remove collected nodes (entire subtrees).
	for _, n := range toRemove {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}

	// 5. Render back to string.
	var buf strings.Builder
	if err := html.Render(&buf, doc); err != nil {
		return removeHiddenElementsRegex(rawHTML)
	}
	return buf.String()
}

// shouldRemoveElement checks if a DOM element should be removed.
// Mirrors OpenClaw's shouldRemoveElement() with full attribute/style/class checks.
func shouldRemoveElement(n *html.Node) bool {
	tag := strings.ToLower(n.Data)

	// Always-remove tags.
	switch tag {
	case "meta", "template", "svg", "canvas", "iframe", "object", "embed":
		return true
	}

	// input type=hidden
	if tag == "input" && getAttr(n, "type") == "hidden" {
		return true
	}

	// aria-hidden="true"
	if getAttr(n, "aria-hidden") == "true" {
		return true
	}

	// hidden attribute
	if hasAttr(n, "hidden") {
		return true
	}

	// Class-based hiding.
	className := getAttr(n, "class")
	if className != "" && hasHiddenClass(className) {
		return true
	}

	// Inline style-based hiding.
	style := getAttr(n, "style")
	if style != "" && isStyleHidden(style) {
		return true
	}

	return false
}

// hiddenClassNames matches OpenClaw's HIDDEN_CLASS_NAMES set.
var hiddenClassNames = map[string]bool{
	"sr-only":            true,
	"visually-hidden":    true,
	"d-none":             true,
	"hidden":             true,
	"invisible":          true,
	"screen-reader-only": true,
	"offscreen":          true,
}

func hasHiddenClass(className string) bool {
	for _, cls := range strings.Fields(strings.ToLower(className)) {
		if hiddenClassNames[cls] {
			return true
		}
	}
	return false
}

// isStyleHidden checks inline style for hidden patterns.
// Mirrors OpenClaw's full HIDDEN_STYLE_PATTERNS + extra checks.
func isStyleHidden(style string) bool {
	lower := strings.ToLower(style)

	// display: none
	if reStyleDisplayNone.MatchString(lower) {
		return true
	}
	// visibility: hidden
	if reStyleVisibilityHidden.MatchString(lower) {
		return true
	}
	// opacity: 0
	if reStyleOpacity0.MatchString(lower) {
		return true
	}
	// font-size: 0
	if reStyleFontSize0.MatchString(lower) {
		return true
	}
	// text-indent: -9999px (large negative)
	if reStyleTextIndentNeg.MatchString(lower) {
		return true
	}
	// color: transparent
	if reStyleColorTransparent.MatchString(lower) {
		return true
	}
	// color: rgba(..., 0)
	if reStyleColorRGBA0.MatchString(lower) {
		return true
	}
	// clip-path: inset() with positive percentage
	if m := reStyleClipPath.FindStringSubmatch(lower); len(m) > 1 {
		if !reNone.MatchString(m[1]) && reClipPathInsetPct.MatchString(m[1]) {
			return true
		}
	}
	// transform: scale(0)
	if m := reStyleTransform.FindStringSubmatch(lower); len(m) > 1 {
		if reTransformScale0.MatchString(m[1]) {
			return true
		}
		if reTransformTranslateXNeg.MatchString(m[1]) {
			return true
		}
		if reTransformTranslateYNeg.MatchString(m[1]) {
			return true
		}
	}
	// width:0 + height:0 + overflow:hidden
	if reStyleWidth0.MatchString(lower) && reStyleHeight0.MatchString(lower) && reStyleOverflowHidden.MatchString(lower) {
		return true
	}
	// Offscreen: left/top far negative
	if reStyleLeftNeg.MatchString(lower) {
		return true
	}
	if reStyleTopNeg.MatchString(lower) {
		return true
	}

	return false
}

// Style detection regexes — matching OpenClaw's HIDDEN_STYLE_PATTERNS.
var (
	reStyleDisplayNone       = regexp.MustCompile(`(?:^|;)\s*display\s*:\s*none\s*(?:;|$)`)
	reStyleVisibilityHidden  = regexp.MustCompile(`(?:^|;)\s*visibility\s*:\s*hidden\s*(?:;|$)`)
	reStyleOpacity0          = regexp.MustCompile(`(?:^|;)\s*opacity\s*:\s*0\s*(?:;|$)`)
	reStyleFontSize0         = regexp.MustCompile(`(?:^|;)\s*font-size\s*:\s*0(?:px|em|rem|pt|%)?\s*(?:;|$)`)
	reStyleTextIndentNeg     = regexp.MustCompile(`(?:^|;)\s*text-indent\s*:\s*-\d{4,}px`)
	reStyleColorTransparent  = regexp.MustCompile(`(?:^|;)\s*color\s*:\s*transparent\s*(?:;|$)`)
	reStyleColorRGBA0        = regexp.MustCompile(`(?:^|;)\s*color\s*:\s*rgba\s*\([^)]*,\s*0(?:\.0+)?\s*\)`)
	reStyleClipPath          = regexp.MustCompile(`(?:^|;)\s*clip-path\s*:\s*([^;]+)`)
	reClipPathInsetPct       = regexp.MustCompile(`inset\s*\(\s*(?:0*\.\d+|[1-9]\d*(?:\.\d+)?)%`)
	reNone                   = regexp.MustCompile(`^\s*none\s*$`)
	reStyleTransform         = regexp.MustCompile(`(?:^|;)\s*transform\s*:\s*([^;]+)`)
	reTransformScale0        = regexp.MustCompile(`scale\s*\(\s*0\s*\)`)
	reTransformTranslateXNeg = regexp.MustCompile(`translatex\s*\(\s*-\d{4,}px\s*\)`)
	reTransformTranslateYNeg = regexp.MustCompile(`translatey\s*\(\s*-\d{4,}px\s*\)`)
	reStyleWidth0            = regexp.MustCompile(`(?:^|;)\s*width\s*:\s*0(?:px)?\s*(?:;|$)`)
	reStyleHeight0           = regexp.MustCompile(`(?:^|;)\s*height\s*:\s*0(?:px)?\s*(?:;|$)`)
	reStyleOverflowHidden    = regexp.MustCompile(`(?:^|;)\s*overflow\s*:\s*hidden`)
	reStyleLeftNeg           = regexp.MustCompile(`(?:^|;)\s*left\s*:\s*-\d{4,}px`)
	reStyleTopNeg            = regexp.MustCompile(`(?:^|;)\s*top\s*:\s*-\d{4,}px`)
)

// DOM helpers.
func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return true
		}
	}
	return false
}

// removeHiddenElementsRegex is the regex fallback when DOM parsing fails.
func removeHiddenElementsRegex(htmlStr string) string {
	re := regexp.MustCompile(`(?is)<(\w+)([^>]*?)>`)
	reHiddenAttr := regexp.MustCompile(`(?i)\s(?:hidden|aria-hidden\s*=\s*["']true["'])`)
	return re.ReplaceAllStringFunc(htmlStr, func(match string) string {
		sub := re.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		attrs := sub[2]
		if reHiddenAttr.MatchString(attrs) {
			return ""
		}
		if strings.Contains(strings.ToLower(attrs), "style=") && reStyleDisplayNone.MatchString(strings.ToLower(attrs)) {
			return ""
		}
		return match
	})
}

// ---------- HTML → Markdown ----------

var (
	reScript   = regexp.MustCompile(`(?is)<script[\s\S]*?</script>`)
	reStyle    = regexp.MustCompile(`(?is)<style[\s\S]*?</style>`)
	reNoScript = regexp.MustCompile(`(?is)<noscript[\s\S]*?</noscript>`)
	reComment  = regexp.MustCompile(`<!--[\s\S]*?-->`)
	reAnchor   = regexp.MustCompile(`(?is)<a\s+[^>]*href=["']([^"']+)["'][^>]*>([\s\S]*?)</a>`)
	reHeading  = regexp.MustCompile(`(?is)<h([1-6])[^>]*>([\s\S]*?)</h[1-6]>`)
	reListItem = regexp.MustCompile(`(?is)<li[^>]*>([\s\S]*?)</li>`)
	reBrHr     = regexp.MustCompile(`(?i)<(?:br|hr)\s*/?>`)
	reBlockEnd = regexp.MustCompile(`(?i)</(?:p|div|section|article|header|footer|table|tr|ul|ol)>`)
	reTags     = regexp.MustCompile(`<[^>]+>`)
	reTitle    = regexp.MustCompile(`(?is)<title[^>]*>([\s\S]*?)</title>`)
	reMultiNL  = regexp.MustCompile(`\n{3,}`)
	reMultiSP  = regexp.MustCompile(`[ \t]{2,}`)
	reTrailWS  = regexp.MustCompile(`[ \t]+\n`)

	// HTML entities.
	reEntHex = regexp.MustCompile(`(?i)&#x([0-9a-f]+);`)
	reEntDec = regexp.MustCompile(`&#(\d+);`)

	// Invisible Unicode (zero-width, bidirectional, tags, etc.).
	// Matches OpenClaw's INVISIBLE_UNICODE_RE.
	reInvisibleUnicode = regexp.MustCompile("[\u200B-\u200F\u202A-\u202E\u2060-\u2064\u206A-\u206F\uFEFF]")

	// Markdown stripping patterns (precompiled for markdownToText).
	reMdImages   = regexp.MustCompile(`!\[[^\]]*]\([^)]+\)`)
	reMdLinks    = regexp.MustCompile(`\[([^\]]+)]\([^)]+\)`)
	reMdFences   = regexp.MustCompile("(?s)```[\\s\\S]*?```")
	reMdInline   = regexp.MustCompile("`([^`]+)`")
	reMdHeadings = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reMdBullets  = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	reMdNumbers  = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
)

// htmlToMarkdown converts raw HTML to a simplified Markdown representation.
func htmlToMarkdown(rawHTML string) (text string, title string) {
	// Extract <title>.
	if m := reTitle.FindStringSubmatch(rawHTML); len(m) > 1 {
		title = normalizeWhitespace(stripTags(m[1]))
	}

	text = rawHTML
	text = reScript.ReplaceAllString(text, "")
	text = reStyle.ReplaceAllString(text, "")
	text = reNoScript.ReplaceAllString(text, "")
	text = reComment.ReplaceAllString(text, "")

	// Anchors → markdown links.
	text = reAnchor.ReplaceAllStringFunc(text, func(match string) string {
		sub := reAnchor.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		href, body := sub[1], sub[2]
		label := normalizeWhitespace(stripTags(body))
		if label == "" {
			return href
		}
		return "[" + label + "](" + href + ")"
	})

	// Headings → markdown headings.
	text = reHeading.ReplaceAllStringFunc(text, func(match string) string {
		sub := reHeading.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		level := sub[1]
		body := sub[2]
		n := 1
		if len(level) == 1 && level[0] >= '1' && level[0] <= '6' {
			n = int(level[0] - '0')
		}
		prefix := strings.Repeat("#", n)
		label := normalizeWhitespace(stripTags(body))
		return "\n" + prefix + " " + label + "\n"
	})

	// List items → markdown bullets.
	text = reListItem.ReplaceAllStringFunc(text, func(match string) string {
		sub := reListItem.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		label := normalizeWhitespace(stripTags(sub[1]))
		if label == "" {
			return ""
		}
		return "\n- " + label
	})

	text = reBrHr.ReplaceAllString(text, "\n")
	text = reBlockEnd.ReplaceAllString(text, "\n")
	text = stripTags(text)
	text = normalizeWhitespace(text)
	return text, title
}

// markdownToText strips markdown formatting, returning plain text.
func markdownToText(md string) string {
	text := md
	text = reMdImages.ReplaceAllString(text, "")
	text = reMdLinks.ReplaceAllString(text, "$1")
	text = reMdFences.ReplaceAllStringFunc(text, func(block string) string {
		inner := strings.TrimPrefix(block, "```")
		inner = strings.TrimSuffix(inner, "```")
		if idx := strings.IndexByte(inner, '\n'); idx >= 0 {
			inner = inner[idx+1:]
		}
		return inner
	})
	text = reMdInline.ReplaceAllString(text, "$1")
	text = reMdHeadings.ReplaceAllString(text, "")
	text = reMdBullets.ReplaceAllString(text, "")
	text = reMdNumbers.ReplaceAllString(text, "")
	return normalizeWhitespace(text)
}

// stripInvisibleUnicode removes zero-width and invisible Unicode characters.
func stripInvisibleUnicode(text string) string {
	return reInvisibleUnicode.ReplaceAllString(text, "")
}

// ---------- Helpers ----------

func stripTags(s string) string {
	s = reTags.ReplaceAllString(s, "")
	return decodeEntities(s)
}

func decodeEntities(s string) string {
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = reEntHex.ReplaceAllStringFunc(s, func(m string) string {
		sub := reEntHex.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		var n int
		for _, c := range sub[1] {
			n *= 16
			switch {
			case c >= '0' && c <= '9':
				n += int(c - '0')
			case c >= 'a' && c <= 'f':
				n += int(c-'a') + 10
			case c >= 'A' && c <= 'F':
				n += int(c-'A') + 10
			}
		}
		if n > 0 && utf8.ValidRune(rune(n)) {
			return string(rune(n))
		}
		return m
	})
	s = reEntDec.ReplaceAllStringFunc(s, func(m string) string {
		sub := reEntDec.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		var n int
		for _, c := range sub[1] {
			n = n*10 + int(c-'0')
		}
		if n > 0 && utf8.ValidRune(rune(n)) {
			return string(rune(n))
		}
		return m
	})
	return s
}

func normalizeWhitespace(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = reTrailWS.ReplaceAllString(s, "\n")
	s = reMultiNL.ReplaceAllString(s, "\n\n")
	s = reMultiSP.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func truncateText(text string, maxChars int) (string, bool) {
	if len(text) <= maxChars {
		return text, false
	}
	return text[:maxChars], true
}

func looksLikeHTML(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	head := strings.ToLower(trimmed)
	if len(head) > 256 {
		head = head[:256]
	}
	return strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html")
}
