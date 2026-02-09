package entity

import (
	"fmt"
)

type ModelMeta struct {
	DisplayInfo     *DisplayInfo      `json:"display_info,omitempty"`
	Capability      *ModelAbility     `json:"capability,omitempty"`
	Connection      *Connection       `json:"connection,omitempty"`
	Parameters      []*ModelParameter `json:"parameters,omitempty"`
	EnableBase64URL bool              `json:"enable_base64_url,omitempty"`
}

type ModelType int64

const (
	ModelType_LLM           ModelType = 0
	ModelType_TextEmbedding ModelType = 1
	ModelType_Rerank        ModelType = 2
)

func (p ModelType) String() string {
	switch p {
	case ModelType_LLM:
		return "LLM"
	case ModelType_TextEmbedding:
		return "TextEmbedding"
	case ModelType_Rerank:
		return "Rerank"
	}
	return "<UNSET>"
}

func (p ModelType) Int32() int32 {
	return int32(p)
}

func ModelTypeFromString(s string) (ModelType, error) {
	switch s {
	case "LLM":
		return ModelType_LLM, nil
	case "TextEmbedding":
		return ModelType_TextEmbedding, nil
	case "Rerank":
		return ModelType_Rerank, nil
	}
	return ModelType(0), fmt.Errorf("not a valid ModelType string")
}
