package prompt

import (
	"context"
	"sort"
	"strings"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// Pipeline is the interface that wraps the basic methods of a prompt pipeline.
type Pipeline struct {
	sections        []PromptSection
	mutators        []PromptMutator
	sorted          bool
	workspaceLoader *WorkspaceLoader
}

// NewPipeline creates an empty prompt pipeline.
func NewPipeline() *Pipeline {
	return &Pipeline{}
}

// RegisterSection adds a PromptSection to the pipeline.
// Sections are sorted by Priority before first assembly.
func (p *Pipeline) RegisterSection(s PromptSection) {
	p.sections = append(p.sections, s)
	p.sorted = false
}

// RegisterMutator adds a PromptMutator to the pipeline.
// Mutators are sorted by Priority before first assembly.
func (p *Pipeline) RegisterMutator(m PromptMutator) {
	p.mutators = append(p.mutators, m)
	p.sorted = false
}

// SetWorkspaceLoader attaches a WorkspaceLoader to the pipeline.
// When set, the loader's dynamic sections are merged at assemble time.
// The WorkspaceLoader provides sections from SOUL.md, IDENTITY.md, AGENTS.md
// and any .md files under the workspace's prompts/ subdirectory.
func (p *Pipeline) SetWorkspaceLoader(wl *WorkspaceLoader) {
	p.workspaceLoader = wl
}

// WorkspaceLoader returns the attached WorkspaceLoader (may be nil).
func (p *Pipeline) GetWorkspaceLoader() *WorkspaceLoader {
	return p.workspaceLoader
}

// ensureSorted sorts sections and mutators by priority.
// Called once lazily before first assembly.
func (p *Pipeline) ensureSorted() {
	if p.sorted {
		return
	}
	sort.Slice(p.sections, func(i, j int) bool {
		return p.sections[i].Priority() < p.sections[j].Priority()
	})
	sort.Slice(p.mutators, func(i, j int) bool {
		return p.mutators[i].Priority() < p.mutators[j].Priority()
	})
	p.sorted = true
}

// priorityThreshold defines the maximum section priority included for each PromptMode.
// Sections with priority above the threshold are excluded.
//
//   - PromptModeNone:    only priority <= 100 (identity line)
//   - PromptModeMinimal: only priority <= 500 (core sections)
//   - PromptModeFull:    all sections (no limit)
func priorityThreshold(mode PromptMode) int {
	switch mode {
	case PromptModeNone:
		return 100
	case PromptModeMinimal:
		return 500
	default:
		return 999999
	}
}

// Assemble executes the full prompt assembly pipeline and returns the system prompt text.
//
// Flow:
//  1. Merge static sections + dynamic workspace sections
//  2. Sort all sections/mutators by priority (lazy, once for static; always for merged)
//  3. For each section: check Enabled + PromptMode threshold → Render
//  4. Apply mutators in order
//  5. Return final assembled text
//
// Individual section failures are logged and skipped (K8s failurePolicy: Ignore).
func (p *Pipeline) Assemble(ctx context.Context, pc *PromptContext) (string, error) {
	p.ensureSorted()

	// Merge workspace sections dynamically (they may change at runtime via fsnotify).
	allSections := p.sections
	if p.workspaceLoader != nil {
		wsSections := p.workspaceLoader.Sections()
		if len(wsSections) > 0 {
			allSections = make([]PromptSection, 0, len(p.sections)+len(wsSections))
			allSections = append(allSections, p.sections...)
			allSections = append(allSections, wsSections...)
			// Re-sort the merged list.
			sort.Slice(allSections, func(i, j int) bool {
				return allSections[i].Priority() < allSections[j].Priority()
			})
		}
	}

	threshold := priorityThreshold(pc.Mode)
	var buf strings.Builder

	for _, section := range allSections {
		// PromptMode filter: skip sections above the threshold.
		if section.Priority() > threshold {
			continue
		}

		// Dynamic enable check.
		if !section.Enabled(ctx, pc) {
			continue
		}

		text, err := section.Render(ctx, pc)
		if err != nil {
			// K8s failurePolicy: Ignore — log and continue.
			logger.Warn("[PromptPipeline] section %q render failed: %v", section.Name(), err)
			continue
		}
		if text == "" {
			continue
		}

		buf.WriteString(text)
		buf.WriteString("\n\n")
	}

	result := strings.TrimRight(buf.String(), "\n")

	// Mutator chain.
	for _, m := range p.mutators {
		mutated, err := m.Mutate(ctx, pc, result)
		if err != nil {
			logger.Warn("[PromptPipeline] mutator %q failed: %v", m.Name(), err)
			continue
		}
		result = mutated
	}

	return result, nil
}

// SectionCount returns the number of registered sections.
func (p *Pipeline) SectionCount() int {
	return len(p.sections)
}

// MutatorCount returns the number of registered mutators.
func (p *Pipeline) MutatorCount() int {
	return len(p.mutators)
}
