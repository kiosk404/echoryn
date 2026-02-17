package service

import (
	"context"
	"fmt"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/repo"
	"github.com/kiosk404/echoryn/pkg/logger"
)

type FallbackExecutor struct {
	modelRepo repo.ModelRepository
	manager   ModelManager
}

// NewFallbackExecutor creates a new FallbackExecutor.
func NewFallbackExecutor(modelRepo repo.ModelRepository, manager ModelManager) *FallbackExecutor {
	return &FallbackExecutor{
		modelRepo: modelRepo,
		manager:   manager,
	}
}

// RunFunc is the function signature for the operation to execute with fallback.
// It receives a BaseChatModel (Generate/Stream) and returns a result or error.
// Uses BaseChatModel instead of the deprecated ChatModel to avoid BindTools
// concurrency issues. Callers needing tool-calling should assert ToolCallingChatModel.
type RunFunc[T any] func(ctx context.Context, cm einoModel.BaseChatModel) (T, error)

// OnErrorFunc is a callback invoked on each failed attempt.
type OnErrorFunc func(attempt entity.FallbackAttempt, attemptNum, total int)

// RunWithFallback executes the given function with model fallback.
// It tries each candidate in order according to FallbackConfig.
// If all candidates fail, returns a combined error with all attempts.
//
// Modeled after OpenClaw's runWithModelFallback<T>().
func RunWithFallback[T any](
	ctx context.Context,
	executor *FallbackExecutor,
	config entity.FallbackConfig,
	params *entity.LLMParams,
	run RunFunc[T],
	onError OnErrorFunc,
) *entity.FallbackResult[T] {
	candidates := config.Candidates()
	maxAttempts := config.EffectiveMaxAttempts()

	result := &entity.FallbackResult[T]{
		Attempts: make([]entity.FallbackAttempt, 0, len(candidates)),
	}

	for i, ref := range candidates {
		if i >= maxAttempts {
			break
		}

		// Check cooldown status if configured.
		if config.SkipOnCooldown {
			if instance, err := executor.modelRepo.FindByRef(ctx, ref); err == nil {
				if instance.Status == entity.ModelStatus_CoolDown {
					attempt := entity.FallbackAttempt{
						Ref:        ref,
						Skipped:    true,
						SkipReason: fmt.Sprintf("provider %s is in cooldown", ref.ProviderID),
						Reason:     entity.FailoverReason_RateLimit,
					}
					result.Attempts = append(result.Attempts, attempt)
					logger.Info("[Fallback] skipping %s (cooldown)", ref)
					continue
				}
			}
		}

		// Build ChatModel for this candidate.
		cm, err := executor.manager.BuildChatModel(ctx, ref, params)
		if err != nil {
			attempt := entity.FallbackAttempt{
				Ref:    ref,
				Error:  err.Error(),
				Reason: entity.ClassifyError(err),
			}
			result.Attempts = append(result.Attempts, attempt)
			logger.Warn("[Fallback] failed to build model %s: %v", ref, err)
			if onError != nil {
				onError(attempt, i+1, len(candidates))
			}
			continue
		}

		// Execute the operation.
		value, err := run(ctx, cm)
		if err != nil {
			// Classify the error.
			fe := entity.NewFailoverErrorFromCause(err, ref.ProviderID, ref.ModelID)

			attempt := entity.FallbackAttempt{
				Ref:        ref,
				Error:      fe.Message,
				Reason:     fe.Reason,
				StatusCode: fe.StatusCode,
			}
			result.Attempts = append(result.Attempts, attempt)

			logger.Warn("[Fallback] attempt %d/%d failed (%s): %s [reason=%s]",
				i+1, len(candidates), ref, fe.Message, fe.Reason)

			if onError != nil {
				onError(attempt, i+1, len(candidates))
			}

			// Check if we should abort (non-failover errors).
			if !fe.Reason.ShouldFailover() {
				break
			}
			continue
		}

		// Success!
		result.Value = value
		result.Ref = ref
		result.OK = true
		logger.Info("[Fallback] succeeded with %s (attempt %d/%d)", ref, i+1, len(candidates))
		return result
	}

	return result
}

// RunChatWithFallback is a convenience wrapper for common chat completion with fallback.
// It builds each candidate model and runs the provided function.
func (e *FallbackExecutor) RunChatWithFallback(
	ctx context.Context,
	config entity.FallbackConfig,
	params *entity.LLMParams,
	run RunFunc[*entity.ChatCompletionResult],
	onError OnErrorFunc,
) (*entity.FallbackResult[*entity.ChatCompletionResult], error) {
	result := RunWithFallback(ctx, e, config, params, run, onError)
	if !result.OK {
		return result, result.AllFailedError()
	}
	return result, nil
}

// GetChatModelWithFallback tries to get a usable BaseChatModel from the candidate list.
// Returns the first successfully built model and its ref.
// Returns BaseChatModel; callers needing ToolCallingChatModel can assert the returned value.
func (e *FallbackExecutor) GetChatModelWithFallback(
	ctx context.Context,
	config entity.FallbackConfig,
	params *entity.LLMParams,
) (einoModel.BaseChatModel, entity.ModelRef, error) {
	candidates := config.Candidates()

	for i, ref := range candidates {
		if config.SkipOnCooldown {
			if instance, err := e.modelRepo.FindByRef(ctx, ref); err == nil {
				if instance.Status == entity.ModelStatus_CoolDown {
					logger.Info("[Fallback] skipping %s (cooldown)", ref)
					continue
				}
			}
		}

		cm, err := e.manager.BuildChatModel(ctx, ref, params)
		if err != nil {
			logger.Warn("[Fallback] failed to build model %s (attempt %d/%d): %v",
				ref, i+1, len(candidates), err)
			continue
		}

		return cm, ref, nil
	}

	return nil, entity.ModelRef{}, fmt.Errorf("no usable model found in %d candidates", len(candidates))
}
