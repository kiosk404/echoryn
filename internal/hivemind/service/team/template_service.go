package team

import (
	"context"
	"fmt"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// templateService is the default implementation of TeamTemplateService.
// It delegates persistence to TeamRegistry and handles validation,
type templateService struct {
	registry TeamRegistry
}

// NewTemplateService creates a new TeamTemplateService.
func NewTemplateService(registry TeamRegistry) TeamTemplateService {
	return &templateService{registry: registry}
}

var _ TeamTemplateService = (*templateService)(nil)

func (s *templateService) CreateTemplate(ctx context.Context, template *TeamTemplate) error {
	if err := template.Validate(); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	// Check for duplicate.
	existing, err := s.registry.GetTemplate(ctx, template.ID)
	if err != nil {
		return fmt.Errorf("failed to check existing template: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("template %s already exists", template.ID)
	}

	now := time.Now()
	template.CreatedAt = now
	template.UpdatedAt = now

	if err := s.registry.SaveTemplate(ctx, template); err != nil {
		return fmt.Errorf("failed to save template: %w", err)
	}

	logger.Info("[TeamTemplateService] created template: id=%s, name=%s, members=%d",
		template.ID, template.Name, len(template.MemberSpecs))
	return nil
}

func (s *templateService) GetTemplate(ctx context.Context, templateID string) (*TeamTemplate, error) {
	t, err := s.registry.GetTemplate(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	return t, nil
}

func (s *templateService) ListTemplates(ctx context.Context, filter *TemplateFilter) ([]*TeamTemplate, error) {
	templates, err := s.registry.ListTemplates(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list templates: %w", err)
	}
	return templates, nil
}

func (s *templateService) UpdateTemplate(ctx context.Context, template *TeamTemplate) error {
	if err := template.Validate(); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}
	existing, err := s.registry.GetTemplate(ctx, template.ID)
	if err != nil {
		return fmt.Errorf("failed to check existing template: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("template %s to found", template.ID)
	}

	template.CreatedAt = existing.CreatedAt
	template.UpdatedAt = time.Now()

	if err := s.registry.SaveTemplate(ctx, template); err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}

	logger.Info("[TeamTemplateService] updated template: id=%s, name=%s", template.ID, template.Name)
	return nil
}

func (s *templateService) DeleteTemplate(ctx context.Context, templateID string) error {
	if err := s.registry.DeleteTemplate(ctx, templateID); err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}
	logger.Info("[TeamTemplateService] deleted template: id=%s", templateID)
	return nil
}
