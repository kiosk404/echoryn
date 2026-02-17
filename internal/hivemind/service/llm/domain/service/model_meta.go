package service

import (
	"fmt"

	"github.com/jinzhu/copier"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/pkg"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// ModelMetaConf holds the static metadata configuration for model providers.
// organized as a two-level map: provider -> model_name -> model_meta
type ModelMetaConf struct {
	Provider2Models map[string]map[string]ModelMeta `thrift:"provider2models,2" form:"provider2models" json:"provider2models" query:"provider2models"`
}

// ModelMeta is an alias for entity.ModelMeta, used in configuration context
type ModelMeta entity.ModelMeta

// NewModelMetaConf creates a new ModelMetaConf.
func NewModelMetaConf() *ModelMetaConf {
	return &ModelMetaConf{
		Provider2Models: make(map[string]map[string]ModelMeta),
	}
}

// GetModelMeta retrieves the model metadata for a given provider class and model name.
// Falls back to "default" if exact match is not found.
func (c *ModelMetaConf) GetModelMeta(modelClass entity.ModelClass, modelName string) (*ModelMeta, error) {
	modelName2Meta, ok := c.Provider2Models[modelClass.String()]
	if !ok {
		return nil, fmt.Errorf("model meta not found for model class %v", modelClass)
	}

	modelMeta, ok := modelName2Meta[modelName]
	if ok {
		logger.InfoX(pkg.LLMModel, "get model meta for model class %v and model name %v", modelClass, modelName)
		return deepCopyModelMeta(&modelMeta)
	}

	const defaultKey = "default"
	modelMeta, ok = modelName2Meta[defaultKey]
	if ok {
		logger.InfoX(pkg.LLMModel, "use default model meta for model class %v and model name %v", modelClass, modelName)
		return deepCopyModelMeta(&modelMeta)
	}

	return nil, fmt.Errorf("model meta not found for model class %v and model name %v", modelClass, modelName)
}

// SetModelMeta sets the model metadata for a given provider class and model name.
func (c *ModelMetaConf) SetModelMeta(modelClass entity.ModelClass, modelName string, meta ModelMeta) {
	modelName2Meta, ok := c.Provider2Models[modelClass.String()]
	if !ok {
		modelName2Meta = make(map[string]ModelMeta)
		c.Provider2Models[modelClass.String()] = modelName2Meta
	}
	modelName2Meta[modelName] = meta
}

func deepCopyModelMeta(meta *ModelMeta) (*ModelMeta, error) {
	if meta == nil {
		return nil, nil
	}
	newObj := &ModelMeta{}
	err := copier.CopyWithOption(newObj, meta, copier.Option{DeepCopy: true, IgnoreEmpty: true})
	if err != nil {
		return nil, fmt.Errorf("error copy model meta: %w", err)
	}

	return newObj, nil
}
