package internal

import (
	"math"

	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// CosineSimilarity calculates the cosine similarity between two float32 vectors.
//
// The cosine similarity is defined as the dot product of the two vectors
// divided by the product of their magnitudes.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	length := len(a)
	if len(b) < length {
		length = len(b)
	}

	var dot, normA, normB float64
	for i := 0; i < length; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// ParseEmbedding parses a JSON-encoded string into a slice of float32 values.
//
// If the parsing fails, an error is logged and nil is returned.
func ParseEmbedding(text string) []float32 {
	var embeddings []float32
	if err := json.Unmarshal([]byte(text), &embeddings); err != nil {
		logger.Error("failed to parse embedding: %v", err)
		return nil
	}
	return embeddings
}

// TruncateUTF8Safe truncates a string to a maximum number of UTF-8 characters.
//
// If the string is shorter than or equal to the maximum number of characters,
// it is returned as is.
func TruncateUTF8Safe(s string, maxChars int) string {
	if maxChars <= 0 {
		return s
	}
	runes := []rune(s)
	if len(s) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}
