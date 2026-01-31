// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package embedding provides an ONNX-based embedding engine for semantic matching.
// It uses the MiniLM model to generate 384-dimensional embeddings for text.
package embedding

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

	log "github.com/sirupsen/logrus"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	// DefaultModelName is the default embedding model to use
	DefaultModelName = "all-MiniLM-L6-v2"

	// EmbeddingDimension is the output dimension of the MiniLM model
	EmbeddingDimension = 384

	// MaxSequenceLength is the maximum input sequence length
	MaxSequenceLength = 256
)

// Engine provides embedding inference using ONNX runtime.
// It loads a MiniLM model and provides methods for computing embeddings.
type Engine struct {
	// session is the ONNX runtime session
	session *ort.DynamicAdvancedSession

	// modelPath is the path to the ONNX model file
	modelPath string

	// vocabPath is the path to the vocabulary file
	vocabPath string

	// tokenizer handles text tokenization
	tokenizer *SimpleTokenizer

	// dimension is the embedding output dimension
	dimension int

	// enabled indicates whether the engine is ready
	enabled bool

	// mu protects concurrent access
	mu sync.RWMutex
}

// Config holds configuration for the embedding engine.
type Config struct {
	// ModelPath is the path to the ONNX model file
	ModelPath string

	// VocabPath is the path to the vocabulary file
	VocabPath string

	// SharedLibraryPath is the path to the ONNX runtime shared library
	SharedLibraryPath string
}

// NewEngine creates a new embedding engine with the given configuration.
// The engine is not initialized until Initialize() is called.
//
// Parameters:
//   - cfg: Configuration for the engine
//
// Returns:
//   - *Engine: A new engine instance
//   - error: Any error encountered during creation
func NewEngine(cfg Config) (*Engine, error) {
	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("model path is required")
	}

	return &Engine{
		modelPath: cfg.ModelPath,
		vocabPath: cfg.VocabPath,
		dimension: EmbeddingDimension,
		enabled:   false,
	}, nil
}

// Initialize loads the ONNX model and prepares the engine for inference.
// This must be called before using Embed() or BatchEmbed().
//
// Parameters:
//   - sharedLibPath: Path to the ONNX runtime shared library
//
// Returns:
//   - error: Any error encountered during initialization
func (e *Engine) Initialize(sharedLibPath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if model file exists
	if _, err := os.Stat(e.modelPath); os.IsNotExist(err) {
		return fmt.Errorf("model file not found: %s", e.modelPath)
	}

	// Initialize ONNX runtime
	if sharedLibPath != "" {
		ort.SetSharedLibraryPath(sharedLibPath)
	}

	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("failed to initialize ONNX runtime: %w", err)
	}

	// Create session options
	options, err := ort.NewSessionOptions()
	if err != nil {
		return fmt.Errorf("failed to create session options: %w", err)
	}
	defer options.Destroy()

	// Load the model
	session, err := ort.NewDynamicAdvancedSession(
		e.modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		options,
	)
	if err != nil {
		return fmt.Errorf("failed to load ONNX model: %w", err)
	}
	e.session = session

	// Initialize tokenizer
	tokenizer, err := NewSimpleTokenizer(e.vocabPath)
	if err != nil {
		e.session.Destroy()
		return fmt.Errorf("failed to initialize tokenizer: %w", err)
	}
	e.tokenizer = tokenizer

	e.enabled = true
	log.Infof("Embedding engine initialized with model: %s", filepath.Base(e.modelPath))

	return nil
}

// IsEnabled returns whether the engine is ready for inference.
//
// Returns:
//   - bool: true if the engine is initialized and ready
func (e *Engine) IsEnabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enabled
}

// GetDimension returns the embedding output dimension.
//
// Returns:
//   - int: The embedding dimension (384 for MiniLM)
func (e *Engine) GetDimension() int {
	return e.dimension
}

// Embed computes the embedding vector for a single text.
// The text is tokenized and passed through the model.
//
// Parameters:
//   - text: The input text to embed
//
// Returns:
//   - []float32: The embedding vector (384 dimensions)
//   - error: Any error encountered during embedding
func (e *Engine) Embed(text string) ([]float32, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.enabled {
		return nil, fmt.Errorf("embedding engine not initialized")
	}

	// Tokenize the text
	tokens, err := e.tokenizer.Tokenize(text, MaxSequenceLength)
	if err != nil {
		return nil, fmt.Errorf("tokenization failed: %w", err)
	}

	// Run inference
	embedding, err := e.runInference(tokens)
	if err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	return embedding, nil
}

// BatchEmbed computes embeddings for multiple texts efficiently.
// This is more efficient than calling Embed() multiple times.
//
// Parameters:
//   - texts: The input texts to embed
//
// Returns:
//   - [][]float32: The embedding vectors for each text
//   - error: Any error encountered during embedding
func (e *Engine) BatchEmbed(texts []string) ([][]float32, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.enabled {
		return nil, fmt.Errorf("embedding engine not initialized")
	}

	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	embeddings := make([][]float32, len(texts))

	// Process each text (batch processing can be optimized later)
	for i, text := range texts {
		tokens, err := e.tokenizer.Tokenize(text, MaxSequenceLength)
		if err != nil {
			return nil, fmt.Errorf("tokenization failed for text %d: %w", i, err)
		}

		embedding, err := e.runInference(tokens)
		if err != nil {
			return nil, fmt.Errorf("inference failed for text %d: %w", i, err)
		}

		embeddings[i] = embedding
	}

	return embeddings, nil
}

// runInference executes the ONNX model with the given tokens.
// Must be called with read lock held.
func (e *Engine) runInference(tokens *TokenizedInput) ([]float32, error) {
	seqLen := int64(len(tokens.InputIDs))

	// Create input tensors
	inputIDsTensor, err := ort.NewTensor(
		ort.NewShape(1, seqLen),
		tokens.InputIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
	}
	defer inputIDsTensor.Destroy()

	attentionMaskTensor, err := ort.NewTensor(
		ort.NewShape(1, seqLen),
		tokens.AttentionMask,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
	}
	defer attentionMaskTensor.Destroy()

	tokenTypeIDsTensor, err := ort.NewTensor(
		ort.NewShape(1, seqLen),
		tokens.TokenTypeIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create token_type_ids tensor: %w", err)
	}
	defer tokenTypeIDsTensor.Destroy()

	// Create output tensor
	outputTensor, err := ort.NewEmptyTensor[float32](
		ort.NewShape(1, seqLen, int64(e.dimension)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	// Run inference
	err = e.session.Run(
		[]ort.ArbitraryTensor{inputIDsTensor, attentionMaskTensor, tokenTypeIDsTensor},
		[]ort.ArbitraryTensor{outputTensor},
	)
	if err != nil {
		return nil, fmt.Errorf("ONNX inference failed: %w", err)
	}

	// Extract output and apply mean pooling
	output := outputTensor.GetData()
	embedding := e.meanPooling(output, tokens.AttentionMask, int(seqLen))

	// Normalize the embedding
	embedding = e.normalize(embedding)

	return embedding, nil
}

// meanPooling applies mean pooling over the sequence dimension.
// This averages the token embeddings, weighted by the attention mask.
func (e *Engine) meanPooling(output []float32, attentionMask []int64, seqLen int) []float32 {
	embedding := make([]float32, e.dimension)
	var totalWeight float32

	for i := 0; i < seqLen; i++ {
		if attentionMask[i] == 1 {
			for j := 0; j < e.dimension; j++ {
				embedding[j] += output[i*e.dimension+j]
			}
			totalWeight++
		}
	}

	if totalWeight > 0 {
		for j := 0; j < e.dimension; j++ {
			embedding[j] /= totalWeight
		}
	}

	return embedding
}

// normalize applies L2 normalization to the embedding vector.
func (e *Engine) normalize(embedding []float32) []float32 {
	var norm float32
	for _, v := range embedding {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm > 0 {
		for i := range embedding {
			embedding[i] /= norm
		}
	}

	return embedding
}

// CosineSimilarity computes the cosine similarity between two embedding vectors.
// Both vectors should be normalized for accurate results.
//
// Parameters:
//   - a: First embedding vector
//   - b: Second embedding vector
//
// Returns:
//   - float64: Cosine similarity score (-1.0 to 1.0)
func (e *Engine) CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dotProduct float64
	var normA, normB float64

	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	normA = math.Sqrt(normA)
	normB = math.Sqrt(normB)

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (normA * normB)
}

// Shutdown gracefully shuts down the embedding engine.
// It releases all resources held by the ONNX runtime.
//
// Returns:
//   - error: Any error encountered during shutdown
func (e *Engine) Shutdown() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return nil
	}

	if e.session != nil {
		e.session.Destroy()
		e.session = nil
	}

	e.enabled = false
	log.Info("Embedding engine shut down")

	return nil
}
