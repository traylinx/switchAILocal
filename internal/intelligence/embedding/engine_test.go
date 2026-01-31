// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package embedding

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEngine tests engine creation.
func TestNewEngine(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid config",
			cfg: Config{
				ModelPath: "/path/to/model.onnx",
				VocabPath: "/path/to/vocab.txt",
			},
			wantErr: false,
		},
		{
			name: "missing model path",
			cfg: Config{
				VocabPath: "/path/to/vocab.txt",
			},
			wantErr:   true,
			errSubstr: "model path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewEngine(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, engine)
				assert.Equal(t, EmbeddingDimension, engine.GetDimension())
			}
		})
	}
}

// TestEngineNotInitialized tests that operations fail when engine is not initialized.
func TestEngineNotInitialized(t *testing.T) {
	engine := &Engine{
		enabled:   false,
		dimension: EmbeddingDimension,
	}

	// Embed should fail
	_, err := engine.Embed("test text")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")

	// BatchEmbed should fail
	_, err = engine.BatchEmbed([]string{"test1", "test2"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")

	// IsEnabled should return false
	assert.False(t, engine.IsEnabled())
}

// TestCosineSimilarity tests the cosine similarity computation.
func TestCosineSimilarity(t *testing.T) {
	engine := &Engine{dimension: EmbeddingDimension}

	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		delta    float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 1.0,
			delta:    0.0001,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{0.0, 1.0, 0.0},
			expected: 0.0,
			delta:    0.0001,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{-1.0, 0.0, 0.0},
			expected: -1.0,
			delta:    0.0001,
		},
		{
			name:     "similar vectors",
			a:        []float32{1.0, 1.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 0.7071, // cos(45°)
			delta:    0.001,
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
			delta:    0.0001,
		},
		{
			name:     "different length vectors",
			a:        []float32{1.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 0.0,
			delta:    0.0001,
		},
		{
			name:     "zero vector",
			a:        []float32{0.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 0.0,
			delta:    0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.CosineSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, tt.delta)
		})
	}
}

// TestEmbeddingDimension tests that the embedding dimension is correct.
func TestEmbeddingDimension(t *testing.T) {
	assert.Equal(t, 384, EmbeddingDimension)
}

// TestMeanPooling tests the mean pooling function.
func TestMeanPooling(t *testing.T) {
	engine := &Engine{dimension: 3}

	tests := []struct {
		name          string
		output        []float32
		attentionMask []int64
		seqLen        int
		expected      []float32
	}{
		{
			name: "all tokens attended",
			output: []float32{
				1.0, 2.0, 3.0, // token 0
				4.0, 5.0, 6.0, // token 1
			},
			attentionMask: []int64{1, 1},
			seqLen:        2,
			expected:      []float32{2.5, 3.5, 4.5}, // average
		},
		{
			name: "partial attention",
			output: []float32{
				1.0, 2.0, 3.0, // token 0 (attended)
				4.0, 5.0, 6.0, // token 1 (not attended)
			},
			attentionMask: []int64{1, 0},
			seqLen:        2,
			expected:      []float32{1.0, 2.0, 3.0}, // only token 0
		},
		{
			name: "single token",
			output: []float32{
				1.0, 2.0, 3.0,
			},
			attentionMask: []int64{1},
			seqLen:        1,
			expected:      []float32{1.0, 2.0, 3.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.meanPooling(tt.output, tt.attentionMask, tt.seqLen)
			assert.Equal(t, len(tt.expected), len(result))
			for i := range tt.expected {
				assert.InDelta(t, tt.expected[i], result[i], 0.0001)
			}
		})
	}
}

// TestNormalize tests the L2 normalization function.
func TestNormalize(t *testing.T) {
	engine := &Engine{dimension: 3}

	tests := []struct {
		name     string
		input    []float32
		expected []float32
	}{
		{
			name:     "unit vector",
			input:    []float32{1.0, 0.0, 0.0},
			expected: []float32{1.0, 0.0, 0.0},
		},
		{
			name:     "non-unit vector",
			input:    []float32{3.0, 4.0, 0.0},
			expected: []float32{0.6, 0.8, 0.0}, // norm = 5
		},
		{
			name:     "zero vector",
			input:    []float32{0.0, 0.0, 0.0},
			expected: []float32{0.0, 0.0, 0.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.normalize(tt.input)
			assert.Equal(t, len(tt.expected), len(result))
			for i := range tt.expected {
				assert.InDelta(t, tt.expected[i], result[i], 0.0001)
			}

			// Verify the result is normalized (norm = 1 or 0)
			var norm float64
			for _, v := range result {
				norm += float64(v * v)
			}
			norm = math.Sqrt(norm)
			if norm > 0 {
				assert.InDelta(t, 1.0, norm, 0.0001)
			}
		})
	}
}

// TestBatchEmbedEmpty tests batch embedding with empty input.
func TestBatchEmbedEmpty(t *testing.T) {
	engine := &Engine{
		enabled:   true,
		dimension: EmbeddingDimension,
	}

	// Empty batch should return empty result
	result, err := engine.BatchEmbed([]string{})
	assert.NoError(t, err)
	assert.Empty(t, result)
}

// TestGetDimension tests the GetDimension method.
func TestGetDimension(t *testing.T) {
	engine := &Engine{dimension: 384}
	assert.Equal(t, 384, engine.GetDimension())

	engine2 := &Engine{dimension: 768}
	assert.Equal(t, 768, engine2.GetDimension())
}

// TestShutdownNotInitialized tests shutdown when not initialized.
func TestShutdownNotInitialized(t *testing.T) {
	engine := &Engine{enabled: false}
	err := engine.Shutdown()
	assert.NoError(t, err)
}

// TestIntegration tests the full embedding pipeline when ONNX is available.
func TestIntegration(t *testing.T) {
	locator := NewModelLocator()
	if !locator.ModelExists(DefaultModelName) {
		t.Skip("Embedding model not available. Run scripts/download-embedding-model.sh to download.")
	}

	sharedLibPath := locator.GetSharedLibraryPath()
	if sharedLibPath == "" {
		t.Skip("ONNX runtime shared library not found. Install onnxruntime first.")
	}

	cfg := Config{
		ModelPath:         locator.GetModelPath(DefaultModelName),
		VocabPath:         locator.GetVocabPath(DefaultModelName),
		SharedLibraryPath: sharedLibPath,
	}

	engine, err := NewEngine(cfg)
	require.NoError(t, err)

	err = engine.Initialize(sharedLibPath)
	require.NoError(t, err)
	defer engine.Shutdown()

	// Test single embedding
	t.Run("single embedding", func(t *testing.T) {
		embedding, err := engine.Embed("Hello, world!")
		require.NoError(t, err)
		assert.Len(t, embedding, EmbeddingDimension)

		// Verify embedding is normalized
		var norm float64
		for _, v := range embedding {
			norm += float64(v * v)
		}
		norm = math.Sqrt(norm)
		assert.InDelta(t, 1.0, norm, 0.01)
	})

	// Test batch embedding
	t.Run("batch embedding", func(t *testing.T) {
		texts := []string{
			"Hello, world!",
			"How are you?",
			"Write some code",
		}
		embeddings, err := engine.BatchEmbed(texts)
		require.NoError(t, err)
		assert.Len(t, embeddings, len(texts))

		for i, emb := range embeddings {
			assert.Len(t, emb, EmbeddingDimension, "embedding %d has wrong dimension", i)
		}
	})

	// Test semantic similarity
	t.Run("semantic similarity", func(t *testing.T) {
		emb1, err := engine.Embed("Write a Python function")
		require.NoError(t, err)

		emb2, err := engine.Embed("Create a Python method")
		require.NoError(t, err)

		emb3, err := engine.Embed("What is the weather today?")
		require.NoError(t, err)

		// Similar texts should have higher similarity
		sim12 := engine.CosineSimilarity(emb1, emb2)
		sim13 := engine.CosineSimilarity(emb1, emb3)

		t.Logf("Similarity (Python function vs Python method): %.4f", sim12)
		t.Logf("Similarity (Python function vs weather): %.4f", sim13)

		assert.Greater(t, sim12, sim13, "Similar texts should have higher similarity")
	})
}
