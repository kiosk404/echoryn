package options

import (
	"fmt"

	"github.com/spf13/pflag"
)

type ModelOptions struct {
	Mode            string                     `json:"mode" mapstructure:"mode"`
	DefaultProvider string                     `json:"default-provider" mapstructure:"default-provider"`
	DefaultModel    string                     `json:"default-model" mapstructure:"default-model"`
	Providers       map[string]*ProviderConfig `json:"providers" mapstructure:"providers"`
}

type ProviderConfig struct {
	BaseURL    string            `json:"base-url" mapstructure:"base-url"`
	APIKey     string            `json:"api-key" mapstructure:"api-key"`
	API        string            `json:"api" mapstructure:"api"`
	AuthHeader *bool             `json:"auth-header" mapstructure:"auth-header"`
	Headers    map[string]string `json:"headers" mapstructure:"headers"`
	Models     []ModelDefinition `json:"models" mapstructure:"models"`
}

type ModelDefinition struct {
	ID            string            `json:"id" mapstructure:"id"`
	Name          string            `json:"name" mapstructure:"name"`
	API           string            `json:"api" mapstructure:"api"`
	Reasoning     bool              `json:"reasoning" mapstructure:"reasoning"`
	Input         []string          `json:"input" mapstructure:"input"`
	Cost          ModelCost         `json:"cost" mapstructure:"cost"`
	ContextWindow int               `json:"context-window" mapstructure:"context-window"`
	MaxTokens     int               `json:"max-tokens" mapstructure:"max-tokens"`
	Headers       map[string]string `json:"headers" mapstructure:"headers"`
}

type ModelCost struct {
	Input      float64 `json:"input" mapstructure:"input"`
	Output     float64 `json:"output" mapstructure:"output"`
	CacheRead  float64 `json:"cache-read" mapstructure:"cache-read"`
	CacheWrite float64 `json:"cache-write" mapstructure:"cache-write"`
}

func NewModelOptions() *ModelOptions {
	return &ModelOptions{
		Mode:      "merge",
		Providers: make(map[string]*ProviderConfig),
	}
}

func (o *ModelOptions) Validate() []error {
	var errs []error
	if o.Mode != "merge" && o.Mode != "replace" {
		errs = append(errs, fmt.Errorf("invalid model mode %q, must be 'merge' or 'replace'", o.Mode))
	}
	for id, p := range o.Providers {
		if p.BaseURL == "" {
			errs = append(errs, fmt.Errorf("provider %q, base_url is required", id))
		}
		if len(p.Models) == 0 {
			errs = append(errs, fmt.Errorf("provider %q, at least one model is required", id))
		}
		for _, m := range p.Models {
			if m.ID == "" {
				errs = append(errs, fmt.Errorf("provider %q: model id is required", id))
			}
		}
	}
	return errs
}

func (o *ModelOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Mode, "models.mode", o.Mode, "Model provider merge mode: 'merge' or 'replace'.")
	fs.StringVar(&o.DefaultProvider, "models.default-provider", o.DefaultProvider, "Default provider ID.")
	fs.StringVar(&o.DefaultModel, "models.default-model", o.DefaultModel, "Default model ID.")
}
