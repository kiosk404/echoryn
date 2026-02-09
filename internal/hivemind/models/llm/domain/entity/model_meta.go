package entity

import (
	"fmt"
)

type EmbeddingType int64

const (
	EmbeddingType_Ark    EmbeddingType = 0
	EmbeddingType_OpenAI EmbeddingType = 1
	EmbeddingType_Ollama EmbeddingType = 2
	EmbeddingType_Gemini EmbeddingType = 3
	EmbeddingType_HTTP   EmbeddingType = 4
)

func (p EmbeddingType) String() string {
	switch p {
	case EmbeddingType_Ark:
		return "Ark"
	case EmbeddingType_OpenAI:
		return "OpenAI"
	case EmbeddingType_Ollama:
		return "Ollama"
	case EmbeddingType_Gemini:
		return "Gemini"
	case EmbeddingType_HTTP:
		return "HTTP"
	}
	return "<UNSET>"
}

func EmbeddingTypeFromString(s string) (EmbeddingType, error) {
	switch s {
	case "Ark":
		return EmbeddingType_Ark, nil
	case "OpenAI":
		return EmbeddingType_OpenAI, nil
	case "Ollama":
		return EmbeddingType_Ollama, nil
	case "Gemini":
		return EmbeddingType_Gemini, nil
	case "HTTP":
		return EmbeddingType_HTTP, nil
	}
	return EmbeddingType(0), fmt.Errorf("not a valid EmbeddingType string")
}
