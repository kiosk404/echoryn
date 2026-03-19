package feishu

// RenderMode specifies the output format of the markdown rendering pipeline.
type RenderMode int

const (
	// RenderModePost renders to Feishu Post (rich text) format.
	// Used for plain text conversations without complex elements.
	RenderModePost RenderMode = iota

	// RenderModeCard renders to Feishu Interactive Card format.
	// Used when content contains code blocks, tables, or images.
	RenderModeCard
)

// Transformer is the core abstraction of the markdown processing pipeline.
// Each transformer receives markdown text and returns transformed markdown text.
// Transformers are composable and can be chained in a Pipeline.
//
// Design: Strategy Pattern — different render modes use different transformer chains.
type Transformer interface {
	// Transform processes the markdown text and returns the transformed result.
	// The mode parameter allows transformers to adapt behavior per render mode.
	Transform(text string, mode RenderMode) string
}

// TransformerFunc is an adapter to allow the use of ordinary functions as Transformers.
type TransformerFunc func(text string, mode RenderMode) string

func (f TransformerFunc) Transform(text string, mode RenderMode) string {
	return f(text, mode)
}

// Pipeline chains multiple Transformers and executes them in order.
// It implements the Composite pattern, allowing nested pipelines.
type Pipeline struct {
	transformers []Transformer
}

// NewPipeline creates a new Pipeline with the given transformers.
func NewPipeline(transformers ...Transformer) *Pipeline {
	return &Pipeline{transformers: transformers}
}

// Transform applies all transformers in sequence.
func (p *Pipeline) Transform(text string, mode RenderMode) string {
	for _, t := range p.transformers {
		text = t.Transform(text, mode)
	}
	return text
}

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
// Feishu post supports: text (with href), at, img tags.
type PostBlock struct {
	Tag    string `json:"tag,omitempty"`
	Text   string `json:"text,omitempty"`
	Href   string `json:"href,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

// CardContent represents parsed markdown content for Feishu interactive card.
type CardContent struct {
	Title    string // Extracted from first heading (displayed in card header)
	Markdown string // Card-compatible markdown content
}

// CardElement represents a single element in a Feishu interactive card.
type CardElement struct {
	Tag     string `json:"tag"`
	Content string `json:"content,omitempty"`
}
