package scheduler

import (
	"time"

	"github.com/kiosk404/echoryn/internal/pkg/protocol"
)

// ScheduleRequestBuilder provides a fluent API for constructing ScheduleRequest instances.
type ScheduleRequestBuilder struct {
	request *ScheduleRequest
}

// NewScheduleRequest creates a new ScheduleRequestBuilder with sensible defaults.
func NewScheduleRequest(task *protocol.Task) *ScheduleRequestBuilder {
	return &ScheduleRequestBuilder{
		request: &ScheduleRequest{
			Task:        task,
			Mode:        AIMode,
			RequestedAt: time.Now(),
		},
	}
}

// WithDirectMode switches to direct scheduling, targeting a specific Golem node.
func (b *ScheduleRequestBuilder) WithDirectMode(nodeID string) *ScheduleRequestBuilder {
	b.request.Mode = DirectMode
	b.request.TargetNodeID = nodeID
	return b
}

// WithAIMode switches to AI-driven scheduling (this is the default).
func (b *ScheduleRequestBuilder) WithAIMode() *ScheduleRequestBuilder {
	b.request.Mode = AIMode
	b.request.TargetNodeID = ""
	return b
}

// WithLLMMode switches to LLM-enhanced scheduling.
// The LLM first semantically pre-filters candidate nodes, then the AISelector
// scores the remaining candidates with the six-dimensional model.
func (b *ScheduleRequestBuilder) WithLLMMode() *ScheduleRequestBuilder {
	b.request.Mode = LLMMode
	b.request.TargetNodeID = ""
	return b
}

// WithRequiredCapabilities sets the capabilities the target Golem must advertise.
func (b *ScheduleRequestBuilder) WithRequiredCapabilities(caps ...string) *ScheduleRequestBuilder {
	b.request.RequiredCapabilities = caps
	return b
}

// WithRequiredSkills sets the skills that must be installed on the target Golem.
func (b *ScheduleRequestBuilder) WithRequiredSkills(skills ...string) *ScheduleRequestBuilder {
	b.request.RequiredSkills = skills
	return b
}

// WithRequiredFeatures sets the features that the target Golem must support.
func (b *ScheduleRequestBuilder) WithRequiredFeatures(features ...string) *ScheduleRequestBuilder {
	b.request.RequiredFeatures = features
	return b
}

// WithPreferredTags sets soft preferences for node tags.
func (b *ScheduleRequestBuilder) WithPreferredTags(tags map[string]string) *ScheduleRequestBuilder {
	b.request.PreferredTags = tags
	return b
}

// WithResourceRequirements sets minimum resource thresholds.
func (b *ScheduleRequestBuilder) WithResourceRequirements(req *ResourceRequirements) *ScheduleRequestBuilder {
	b.request.ResourceRequirements = req
	return b
}

// WithHints sets the scheduling hints for the AI selector.
func (b *ScheduleRequestBuilder) WithHints(hints *ScheduleHints) *ScheduleRequestBuilder {
	b.request.Hints = hints
	return b
}

// Build returns the constructed ScheduleRequest.
func (b *ScheduleRequestBuilder) Build() *ScheduleRequest {
	return b.request
}
