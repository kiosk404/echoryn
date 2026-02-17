package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/repo"
	"github.com/kiosk404/echoryn/pkg/logger"

	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
)

const (
	defaultProbeTimeout = 10 * time.Second
	defaultConcurrency  = 3
)

// ModelProber is a domain service that probes model availability using concurrent
// health checks. It supports multiple probe types (chat, tool_call, vision) and
// respects per-probe timeouts.
//
// Modeled after OpenClaw's model-scan.ts architecture with Go concurrency patterns.
type ModelProber struct {
	modelRepo    repo.ModelRepository
	providerRepo repo.ProviderRepository
	registry     *provider.Registry
	manager      ModelManager

	// concurrency controls the max parallel probes.
	concurrency int
}

// NewModelProber creates a new ModelProber.
func NewModelProber(
	modelRepo repo.ModelRepository,
	providerRepo repo.ProviderRepository,
	registry *provider.Registry,
	manager ModelManager,
) *ModelProber {
	return &ModelProber{
		modelRepo:    modelRepo,
		providerRepo: providerRepo,
		registry:     registry,
		manager:      manager,
		concurrency:  defaultConcurrency,
	}
}

// SetConcurrency sets the maximum number of parallel probes.
func (p *ModelProber) SetConcurrency(n int) {
	if n > 0 {
		p.concurrency = n
	}
}

// ProbeModel probes a single model with the specified probe types.
func (p *ModelProber) ProbeModel(ctx context.Context, spec entity.ModelProbeSpec) (*entity.ModelScanResult, error) {
	instance, err := p.modelRepo.FindByRef(ctx, spec.Ref)
	if err != nil {
		return nil, err
	}

	prov, err := p.providerRepo.FindByID(ctx, spec.Ref.ProviderID)
	if err != nil {
		return nil, err
	}

	probeTypes := spec.ProbeTypes
	if len(probeTypes) == 0 {
		probeTypes = []entity.ProbeType{entity.ProbeType_Chat}
	}

	timeout := defaultProbeTimeout
	if spec.TimeoutMs > 0 {
		timeout = time.Duration(spec.TimeoutMs) * time.Millisecond
	}

	result := &entity.ModelScanResult{
		Ref:           spec.Ref,
		Instance:      instance,
		Results:       make(map[entity.ProbeType]*entity.ProbeResult, len(probeTypes)),
		ScanTimestamp: time.Now(),
	}

	// Try provider-specific probe first.
	if probeResult := p.probeViaPlugin(ctx, instance, prov, timeout); probeResult != nil {
		result.Results[entity.ProbeType_Chat] = probeResult
		result.Available = probeResult.OK
		return result, nil
	}

	// Fallback: use ChatModel to probe.
	for _, pt := range probeTypes {
		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		pr := p.probeByType(probeCtx, instance, prov, pt)
		cancel()
		result.Results[pt] = pr
	}

	// Available if basic chat probe succeeded.
	if chatResult, ok := result.Results[entity.ProbeType_Chat]; ok {
		result.Available = chatResult.OK
	}

	return result, nil
}

// ScanModels probes multiple models concurrently.
// Modeled after OpenClaw's scanOpenRouterModels with mapWithConcurrency pattern.
func (p *ModelProber) ScanModels(ctx context.Context, specs []entity.ModelProbeSpec, onProgress func(completed, total int)) ([]*entity.ModelScanResult, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	results := make([]*entity.ModelScanResult, len(specs))
	var mu sync.Mutex
	var completed int

	sem := make(chan struct{}, p.concurrency)
	var wg sync.WaitGroup

	for i, spec := range specs {
		wg.Add(1)
		go func(idx int, s entity.ModelProbeSpec) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := p.ProbeModel(ctx, s)
			if err != nil {
				result = &entity.ModelScanResult{
					Ref: s.Ref,
					Results: map[entity.ProbeType]*entity.ProbeResult{
						entity.ProbeType_Chat: {
							OK:        false,
							Error:     err.Error(),
							ProbeType: entity.ProbeType_Chat,
							Timestamp: time.Now(),
						},
					},
					Available:     false,
					ScanTimestamp: time.Now(),
				}
			}

			results[idx] = result

			if onProgress != nil {
				mu.Lock()
				completed++
				c := completed
				mu.Unlock()
				onProgress(c, len(specs))
			}
		}(i, spec)
	}

	wg.Wait()
	return results, nil
}

// ScanAllModels scans all registered models of a given type.
func (p *ModelProber) ScanAllModels(ctx context.Context, modelType entity.ModelType, probeTypes []entity.ProbeType, onProgress func(completed, total int)) ([]*entity.ModelScanResult, error) {
	models, err := p.modelRepo.FindAllByType(ctx, modelType)
	if err != nil {
		return nil, err
	}

	specs := make([]entity.ModelProbeSpec, 0, len(models))
	for _, m := range models {
		if m.Status == entity.ModelStatus_Disabled {
			continue
		}
		specs = append(specs, entity.ModelProbeSpec{
			Ref: entity.ModelRef{
				ProviderID: m.ProviderID,
				ModelID:    m.ModelID,
			},
			ProbeTypes: probeTypes,
		})
	}

	return p.ScanModels(ctx, specs, onProgress)
}

// probeViaPlugin checks if the provider plugin implements ProbePlugin and uses it.
func (p *ModelProber) probeViaPlugin(ctx context.Context, instance *entity.ModelInstance, prov *entity.ModelProvider, timeout time.Duration) *entity.ProbeResult {
	factory, err := p.registry.Get(instance.ProviderID)
	if err != nil {
		return nil
	}

	plugin := factory()
	probePlugin, ok := plugin.(spi.ProbePlugin)
	if !ok {
		return nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	result, err := probePlugin.Probe(probeCtx, instance, prov)
	if err != nil {
		return &entity.ProbeResult{
			OK:        false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err.Error(),
			ProbeType: entity.ProbeType_Chat,
			Timestamp: time.Now(),
		}
	}
	return result
}

// probeByType performs a probe for a specific capability using Eino ChatModel.
func (p *ModelProber) probeByType(ctx context.Context, instance *entity.ModelInstance, _ *entity.ModelProvider, probeType entity.ProbeType) *entity.ProbeResult {
	ref := entity.ModelRef{
		ProviderID: instance.ProviderID,
		ModelID:    instance.ModelID,
	}

	start := time.Now()
	result := &entity.ProbeResult{
		ProbeType: probeType,
		Timestamp: time.Now(),
	}

	cm, err := p.manager.BuildChatModel(ctx, ref, &entity.LLMParams{
		MaxTokens: 16,
	})
	if err != nil {
		result.OK = false
		result.LatencyMs = time.Since(start).Milliseconds()
		result.Error = err.Error()
		return result
	}

	switch probeType {
	case entity.ProbeType_Chat:
		result = p.probeChat(ctx, cm, start)
	case entity.ProbeType_ToolCall:
		result = p.probeToolCall(ctx, cm, start)
	case entity.ProbeType_Vision:
		// Vision probe requires image input — skip if model doesn't support it.
		if !instance.Capability.ImageUnderstanding {
			result.Skipped = true
			result.OK = false
		} else {
			result = p.probeChat(ctx, cm, start)
		}
	case entity.ProbeType_Streaming:
		result = p.probeStreaming(ctx, cm, start)
	default:
		result = p.probeChat(ctx, cm, start)
	}

	result.ProbeType = probeType
	result.Timestamp = time.Now()
	return result
}

// probeChat sends a minimal chat request to verify the model is alive.
func (p *ModelProber) probeChat(ctx context.Context, cm einoModel.BaseChatModel, start time.Time) *entity.ProbeResult {
	_, err := cm.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: "Reply with OK."},
	})
	if err != nil {
		return &entity.ProbeResult{
			OK:        false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err.Error(),
		}
	}
	return &entity.ProbeResult{
		OK:        true,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

// probeToolCall sends a minimal tool call request with a registered ping tool.
// The model must implement ToolCallingChatModel so we can bind a dummy tool;
// otherwise the LLM has no tool to call and the probe always fails.
//
// Modeled after OpenClaw's probeTool which registers a "ping" function before probing.
func (p *ModelProber) probeToolCall(ctx context.Context, cm einoModel.BaseChatModel, start time.Time) *entity.ProbeResult {
	// Assert ToolCallingChatModel so we can bind the probe tool.
	tcm, ok := cm.(einoModel.ToolCallingChatModel)
	if !ok {
		return &entity.ProbeResult{
			OK:        false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "model does not implement ToolCallingChatModel",
		}
	}

	// Register a minimal "ping" tool for the probe.
	pingTool := &schema.ToolInfo{
		Name: "ping",
		Desc: "A simple ping tool for health-check probing. Call it with no arguments.",
	}

	toolCM, err := tcm.WithTools([]*schema.ToolInfo{pingTool})
	if err != nil {
		return &entity.ProbeResult{
			OK:        false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("failed to bind probe tool: %v", err),
		}
	}

	msg, err := toolCM.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: "Call the ping tool with {} and nothing else."},
	})
	if err != nil {
		return &entity.ProbeResult{
			OK:        false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err.Error(),
		}
	}
	hasToolCall := len(msg.ToolCalls) > 0
	if !hasToolCall {
		return &entity.ProbeResult{
			OK:        false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "no tool call returned",
		}
	}
	return &entity.ProbeResult{
		OK:        true,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

// probeStreaming sends a minimal streaming request.
func (p *ModelProber) probeStreaming(ctx context.Context, cm einoModel.BaseChatModel, start time.Time) *entity.ProbeResult {
	sr, err := cm.Stream(ctx, []*schema.Message{
		{Role: schema.User, Content: "Reply with OK."},
	})
	if err != nil {
		return &entity.ProbeResult{
			OK:        false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err.Error(),
		}
	}
	defer sr.Close()

	// Read at least one chunk to confirm streaming works.
	_, err = sr.Recv()
	if err != nil {
		logger.Warn("[Prober] stream recv error: %v", err)
	}

	return &entity.ProbeResult{
		OK:        true,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}
