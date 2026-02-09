package service

import (
	"context"
	"fmt"

	"github.com/jinzhu/copier"
	"github.com/kiosk404/eidolon/internal/hivemind/models/llm/domain/entity"
	"github.com/kiosk404/eidolon/internal/pkg"
	"github.com/kiosk404/eidolon/internal/pkg/options"
	"github.com/kiosk404/eidolon/pkg/logger"
)

type ModelMetaConf struct {
	Provider2Models map[string]map[string]ModelMeta `thrift:"provider2models,2" form:"provider2models" json:"provider2models" query:"provider2models"`
}

type ModelMeta entity.ModelMeta

var modelMetaConf *ModelMetaConf

func initModelCOnf(ctx context.Context, options *options.ModelOptions) (*ModelMetaConf, error) {
	if modelMetaConf != nil {
		return modelMetaConf, nil
	}
	return nil, nil
}

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
