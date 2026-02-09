package entity

type ModelInstance struct {
	ID          int64
	Type        ModelType
	Provider    ModelProvider
	DisplayInfo DisplayInfo
	IsSelected  bool
	Connection  Connection
	Capability  ModelAbility
	Parameters  []ModelParameter
	Extra       ModelExtra
}

type ModelClass int64

const (
	ModelClass_GPT      ModelClass = 1
	ModelClass_QWen     ModelClass = 2
	ModelClass_Gemini   ModelClass = 3
	ModelClass_DeepSeek ModelClass = 4
	ModelClass_Ollama   ModelClass = 5
	ModelClass_Claude   ModelClass = 6
	ModelClass_Kimi     ModelClass = 7
	ModelClass_GLM      ModelClass = 8
	ModelClass_Other    ModelClass = 999
)

func (p ModelClass) String() string {
	switch p {
	case ModelClass_GPT:
		return "gpt"
	case ModelClass_QWen:
		return "qwen"
	case ModelClass_Gemini:
		return "gemini"
	case ModelClass_DeepSeek:
		return "deepseek"
	case ModelClass_Ollama:
		return "ollama"
	case ModelClass_Claude:
		return "claude"
	case ModelClass_Kimi:
		return "kimi"
	case ModelClass_GLM:
		return "glm"
	case ModelClass_Other:
		return "other"
	}
	return "<UNSET>"
}
