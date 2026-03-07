package trace

import (
	"math/rand"
	"sync"
	"time"
)

// Sampler decides whether a trace should be recorded.
type Sampler interface {
	// ShouldSample returns true if the trace should be recorded.
	ShouldSample(traceID TraceID, spanName string, kind SpanKind) bool
}

// AlwaysSampler records every trace.
type AlwaysSampler struct{}

// ShouldSample always returns true.
func (s *AlwaysSampler) ShouldSample(TraceID, string, SpanKind) bool {
	return true
}

// NeverSampler discards every trace.
type NeverSampler struct{}

func (s *NeverSampler) ShouldSample(TraceID, string, SpanKind) bool {
	return false
}

// ProbabilitySampler samples traces with a configuration probability.
type ProbabilitySampler struct {
	mu   sync.Mutex
	rate float64
	rng  *rand.Rand
}

// NewProbabilitySampler creates a sampler that records traces at the given rate.
// rate must be between 0.0(never) and 1.0(always)
func NewProbabilitySampler(rate float64) *ProbabilitySampler {
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	return &ProbabilitySampler{
		rate: rate,
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ShouldSample returns true with probability equal to the configured rate.
func (s *ProbabilitySampler) ShouldSample(TraceID, string, SpanKind) bool {
	if s.rate >= 1.0 {
		return true
	}
	if s.rate <= 0.0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rng.Float64() < s.rate
}

// NewSampler creates a Sampler from the configured sample rate.
// - 1.0 returns AlwaysSampler
// - 0.0 returns NeverSampler
// - otherwise returns ProbabilitySampler
func NewSampler(rate float64) Sampler {
	switch {
	case rate >= 1.0:
		return &AlwaysSampler{}
	case rate <= 0.0:
		return &NeverSampler{}
	default:
		return NewProbabilitySampler(rate)
	}
}
