package hybrid

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// QueryExpansionResult holds the result of query expansion for FTS search.
type QueryExpansionResult struct {
	// Original is the user's original query (trimmed).
	Original string
	// Keywords is the extracted meaningful keywords.
	Keywords []string
	// Expanded is the FTS-ready query: "original OR keyword1 OR keyword2 ...".
	Expanded string
}

// ExpandQueryForFTS expands a conversational query into FTS-friendly keywords.
// This is used in FTS-only mode (no embedding provider) to improve search results.
//
// Examples:
//   - "that thing we discussed about the API" → keywords: ["discussed", "api"]
//   - "之前讨论的那个方案" → keywords: ["讨", "论", "讨论", "方", "案", "方案"]
//   - "what was the solution for the bug" → keywords: ["solution", "bug"]
//
// Matches OpenClaw's expandQueryForFts.
func ExpandQueryForFTS(query string) QueryExpansionResult {
	original := strings.TrimSpace(query)
	keywords := ExtractKeywords(original)

	expanded := original
	if len(keywords) > 0 {
		expanded = original + " OR " + strings.Join(keywords, " OR ")
	}

	return QueryExpansionResult{
		Original: original,
		Keywords: keywords,
		Expanded: expanded,
	}
}

// ExtractKeywords extracts meaningful keywords from a conversational query.
// It removes stop words across 7 languages and applies language-specific tokenization.
//
// Matches OpenClaw's extractKeywords.
func ExtractKeywords(query string) []string {
	tokens := tokenizeQuery(query)
	var keywords []string
	seen := make(map[string]struct{})

	for _, token := range tokens {
		if isStopWord(token) {
			continue
		}
		if !isValidKeyword(token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		keywords = append(keywords, token)
	}

	return keywords
}

// isValidKeyword checks if a token looks like a meaningful keyword.
func isValidKeyword(token string) bool {
	if token == "" {
		return false
	}
	// Skip very short pure-English words (likely stop words or fragments).
	if pureEnglishPattern.MatchString(token) && utf8.RuneCountInString(token) < 3 {
		return false
	}
	// Skip pure numbers.
	if pureDigitsPattern.MatchString(token) {
		return false
	}
	// Skip tokens that are all punctuation/symbols.
	allPunct := true
	for _, r := range token {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
			allPunct = false
			break
		}
	}
	if allPunct {
		return false
	}
	return true
}

var (
	pureEnglishPattern = regexp.MustCompile(`^[a-zA-Z]+$`)
	pureDigitsPattern  = regexp.MustCompile(`^\d+$`)
	splitPattern       = regexp.MustCompile(`[\s\p{P}]+`)
)

// tokenizeQuery splits text into tokens with language-aware handling.
// Handles English, Chinese (unigram+bigram), Japanese (script separation+bigram), Korean (particle stripping).
func tokenizeQuery(text string) []string {
	var tokens []string
	normalized := strings.ToLower(strings.TrimSpace(text))

	segments := splitPattern.Split(normalized, -1)

	for _, segment := range segments {
		if segment == "" {
			continue
		}

		if containsJapaneseKana(segment) {
			// Japanese: separate by script type (ASCII, katakana, kanji, hiragana).
			tokens = append(tokens, tokenizeJapanese(segment)...)
		} else if containsCJK(segment) {
			// Chinese: character-level unigram + bigram.
			tokens = append(tokens, tokenizeChinese(segment)...)
		} else if containsKorean(segment) {
			// Korean: keep word, strip trailing particles.
			tokens = append(tokens, tokenizeKorean(segment)...)
		} else {
			tokens = append(tokens, segment)
		}
	}

	return tokens
}

// tokenizeChinese extracts CJK character unigrams and bigrams from a segment.
func tokenizeChinese(segment string) []string {
	var tokens []string
	var cjkChars []rune

	for _, r := range segment {
		if isCJKRune(r) {
			cjkChars = append(cjkChars, r)
		}
	}

	// Add unigrams.
	for _, r := range cjkChars {
		tokens = append(tokens, string(r))
	}
	// Add bigrams for better phrase matching.
	for i := 0; i < len(cjkChars)-1; i++ {
		tokens = append(tokens, string(cjkChars[i])+string(cjkChars[i+1]))
	}

	return tokens
}

// tokenizeJapanese separates mixed-script Japanese text into ASCII, katakana, kanji chunks.
// Kanji sequences get bigram treatment (same as Chinese).
func tokenizeJapanese(segment string) []string {
	var tokens []string

	// Extract script-specific chunks.
	// We iterate rune by rune and group by script type.
	type scriptType int
	const (
		stNone scriptType = iota
		stASCII
		stKatakana
		stKanji
		stHiragana
	)

	getType := func(r rune) scriptType {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return stASCII
		}
		if r >= '\u30A0' && r <= '\u30FF' || r == 'ー' {
			return stKatakana
		}
		if isCJKRune(r) {
			return stKanji
		}
		if r >= '\u3040' && r <= '\u309F' {
			return stHiragana
		}
		return stNone
	}

	var currentType scriptType
	var currentRunes []rune

	flush := func() {
		if len(currentRunes) == 0 {
			return
		}
		s := string(currentRunes)
		switch currentType {
		case stKanji:
			// Kanji: add the whole string + bigrams.
			tokens = append(tokens, s)
			for i := 0; i < len(currentRunes)-1; i++ {
				tokens = append(tokens, string(currentRunes[i])+string(currentRunes[i+1]))
			}
		case stHiragana:
			// Only keep hiragana sequences of 2+ chars.
			if len(currentRunes) >= 2 {
				tokens = append(tokens, s)
			}
		default:
			tokens = append(tokens, s)
		}
		currentRunes = nil
	}

	for _, r := range segment {
		t := getType(r)
		if t == stNone {
			flush()
			currentType = stNone
			continue
		}
		if t != currentType {
			flush()
			currentType = t
		}
		currentRunes = append(currentRunes, r)
	}
	flush()

	return tokens
}

// tokenizeKorean handles Korean text: keeps the word and emits particle-stripped stems.
func tokenizeKorean(segment string) []string {
	var tokens []string

	if isStopWord(segment) {
		return nil
	}

	// Check if stripping a trailing particle gives a stop word.
	stem := stripKoreanTrailingParticle(segment)
	stemIsStopWord := stem != "" && isStopWord(stem)

	if !stemIsStopWord {
		tokens = append(tokens, segment)
	}

	// Emit particle-stripped stem if useful.
	if stem != "" && !isStopWord(stem) && isUsefulKoreanStem(stem) {
		tokens = append(tokens, stem)
	}

	return tokens
}

// Korean trailing particles, sorted by descending length for longest-match-first.
var koTrailingParticles = []string{
	"에서", "으로", "에게", "한테", "처럼", "같이", "보다", "까지", "부터", "마다", "밖에", "대로",
	"은", "는", "이", "가", "을", "를", "의", "에", "로", "와", "과", "도", "만",
}

// stripKoreanTrailingParticle removes a trailing particle from a Korean token.
// Returns the stem if a particle was found, or empty string if not.
func stripKoreanTrailingParticle(token string) string {
	for _, particle := range koTrailingParticles {
		if len(token) > len(particle) && strings.HasSuffix(token, particle) {
			return token[:len(token)-len(particle)]
		}
	}
	return ""
}

// isUsefulKoreanStem checks if a particle-stripped Korean stem is meaningful.
func isUsefulKoreanStem(stem string) bool {
	// Korean stems need at least 2 syllables.
	if containsKorean(stem) {
		return utf8.RuneCountInString(stem) >= 2
	}
	// ASCII stems from mixed tokens like "API를" → "api".
	return regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(stem)
}

// --- Unicode detection helpers ---

func containsCJK(s string) bool {
	for _, r := range s {
		if isCJKRune(r) {
			return true
		}
	}
	return false
}

func isCJKRune(r rune) bool {
	return r >= '\u4E00' && r <= '\u9FFF'
}

func containsJapaneseKana(s string) bool {
	for _, r := range s {
		if r >= '\u3040' && r <= '\u30FF' {
			return true
		}
	}
	return false
}

func containsKorean(s string) bool {
	for _, r := range s {
		if (r >= '\uAC00' && r <= '\uD7AF') || (r >= '\u3131' && r <= '\u3163') {
			return true
		}
	}
	return false
}

// --- Stop words across 7 languages ---
// Matches OpenClaw's stop word lists for EN, ZH, JA, KO, ES, PT, AR.

func isStopWord(token string) bool {
	_, ok := allStopWords[token]
	return ok
}

// allStopWords is the union of stop words from all supported languages.
var allStopWords = buildStopWords()

func buildStopWords() map[string]struct{} {
	words := make(map[string]struct{})
	for _, list := range [][]string{stopWordsEN, stopWordsZH, stopWordsJA, stopWordsKO, stopWordsES, stopWordsPT, stopWordsAR} {
		for _, w := range list {
			words[w] = struct{}{}
		}
	}
	return words
}

var stopWordsEN = []string{
	// Articles and determiners
	"a", "an", "the", "this", "that", "these", "those",
	// Pronouns
	"i", "me", "my", "we", "our", "you", "your", "he", "she", "it", "they", "them",
	// Common verbs
	"is", "are", "was", "were", "be", "been", "being",
	"have", "has", "had", "do", "does", "did",
	"will", "would", "could", "should", "can", "may", "might",
	// Prepositions
	"in", "on", "at", "to", "for", "of", "with", "by", "from", "about",
	"into", "through", "during", "before", "after", "above", "below",
	"between", "under", "over",
	// Conjunctions
	"and", "or", "but", "if", "then", "because", "as", "while",
	"when", "where", "what", "which", "who", "how", "why",
	// Time references (vague)
	"yesterday", "today", "tomorrow", "earlier", "later", "recently",
	"ago", "just", "now",
	// Vague references
	"thing", "things", "stuff", "something", "anything", "everything", "nothing",
	// Question/request words
	"please", "help", "find", "show", "get", "tell", "give",
}

var stopWordsZH = []string{
	// Pronouns
	"我", "我们", "你", "你们", "他", "她", "它", "他们",
	"这", "那", "这个", "那个", "这些", "那些",
	// Auxiliary words
	"的", "了", "着", "过", "得", "地", "吗", "呢", "吧", "啊", "呀", "嘛", "啦",
	// Verbs (common, vague)
	"是", "有", "在", "被", "把", "给", "让", "用", "到", "去", "来", "做", "说", "看", "找", "想", "要", "能", "会", "可以",
	// Prepositions and conjunctions
	"和", "与", "或", "但", "但是", "因为", "所以", "如果", "虽然",
	"而", "也", "都", "就", "还", "又", "再", "才", "只",
	// Time (vague)
	"之前", "以前", "之后", "以后", "刚才", "现在", "昨天", "今天", "明天", "最近",
	// Vague references
	"东西", "事情", "事", "什么", "哪个", "哪些", "怎么", "为什么", "多少",
	// Question/request words
	"请", "帮", "帮忙", "告诉",
}

var stopWordsJA = []string{
	// Pronouns and references
	"これ", "それ", "あれ", "この", "その", "あの", "ここ", "そこ", "あそこ",
	// Common auxiliaries / vague verbs
	"する", "した", "して", "です", "ます", "いる", "ある", "なる", "できる",
	// Particles / connectors
	"の", "こと", "もの", "ため", "そして", "しかし", "また", "でも", "から", "まで", "より", "だけ",
	// Question words
	"なぜ", "どう", "何", "いつ", "どこ", "誰", "どれ",
	// Time (vague)
	"昨日", "今日", "明日", "最近", "今", "さっき", "前", "後",
}

var stopWordsKO = []string{
	// Particles
	"은", "는", "이", "가", "을", "를", "의", "에", "에서", "로", "으로",
	"와", "과", "도", "만", "까지", "부터", "한테", "에게", "께", "처럼", "같이", "보다", "마다", "밖에", "대로",
	// Pronouns
	"나", "나는", "내가", "나를", "너", "우리", "저", "저희",
	"그", "그녀", "그들", "이것", "저것", "그것", "여기", "저기", "거기",
	// Common verbs / auxiliaries
	"있다", "없다", "하다", "되다", "이다", "아니다", "보다", "주다", "오다", "가다",
	// Nouns (vague)
	"것", "거", "등", "수", "때", "곳", "중", "분",
	// Adverbs
	"잘", "더", "또", "매우", "정말", "아주", "많이", "너무", "좀",
	// Conjunctions
	"그리고", "하지만", "그래서", "그런데", "그러나", "또는", "그러면",
	// Question words
	"왜", "어떻게", "뭐", "언제", "어디", "누구", "무엇", "어떤",
	// Time (vague)
	"어제", "오늘", "내일", "최근", "지금", "아까", "나중", "전에",
	// Request words
	"제발", "부탁",
}

var stopWordsES = []string{
	"el", "la", "los", "las", "un", "una", "unos", "unas", "este", "esta", "ese", "esa",
	"yo", "me", "mi", "nosotros", "nosotras", "tu", "tus", "usted", "ustedes", "ellos", "ellas",
	"de", "del", "a", "en", "con", "por", "para", "sobre", "entre",
	"y", "o", "pero", "si", "porque", "como",
	"es", "son", "fue", "fueron", "ser", "estar", "haber", "tener", "hacer",
	"ayer", "hoy", "mañana", "antes", "despues", "después", "ahora", "recientemente",
	"que", "qué", "cómo", "cuando", "cuándo", "donde", "dónde", "porqué", "favor", "ayuda",
}

var stopWordsPT = []string{
	"o", "a", "os", "as", "um", "uma", "uns", "umas", "este", "esta", "esse", "essa",
	"eu", "me", "meu", "minha", "nos", "nós", "você", "vocês", "ele", "ela", "eles", "elas",
	"de", "do", "da", "em", "com", "por", "para", "sobre", "entre",
	"e", "ou", "mas", "se", "porque", "como",
	"é", "são", "foi", "foram", "ser", "estar", "ter", "fazer",
	"ontem", "hoje", "amanhã", "antes", "depois", "agora", "recentemente",
	"que", "quê", "quando", "onde", "porquê", "favor", "ajuda",
}

var stopWordsAR = []string{
	"ال", "و", "أو", "لكن", "ثم", "بل",
	"أنا", "نحن", "هو", "هي", "هم", "هذا", "هذه", "ذلك", "تلك", "هنا", "هناك",
	"من", "إلى", "الى", "في", "على", "عن", "مع", "بين", "ل", "ب", "ك",
	"كان", "كانت", "يكون", "تكون", "صار", "أصبح", "يمكن", "ممكن",
	"بالأمس", "امس", "اليوم", "غدا", "الآن", "قبل", "بعد", "مؤخرا",
	"لماذا", "كيف", "ماذا", "متى", "أين", "هل", "من فضلك", "فضلا", "ساعد",
}
