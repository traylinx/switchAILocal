// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package embedding

import (
	"testing"
	"time"
)

// BenchmarkEmbed benchmarks the embedding performance.
// The requirement is <10ms latency for single embedding.
func BenchmarkEmbed(b *testing.B) {
	// Skip if ONNX runtime is not available
	locator := NewModelLocator()
	if !locator.ModelExists(DefaultModelName) {
		b.Skip("Embedding model not available. Run scripts/download-embedding-model.sh to download.")
	}

	sharedLibPath := locator.GetSharedLibraryPath()
	if sharedLibPath == "" {
		b.Skip("ONNX runtime shared library not found. Install onnxruntime first.")
	}

	cfg := Config{
		ModelPath:         locator.GetModelPath(DefaultModelName),
		VocabPath:         locator.GetVocabPath(DefaultModelName),
		SharedLibraryPath: sharedLibPath,
	}

	engine, err := NewEngine(cfg)
	if err != nil {
		b.Fatalf("Failed to create engine: %v", err)
	}

	if err := engine.Initialize(sharedLibPath); err != nil {
		b.Fatalf("Failed to initialize engine: %v", err)
	}
	defer engine.Shutdown()

	testText := "Write a Python function to calculate the factorial of a number"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Embed(testText)
		if err != nil {
			b.Fatalf("Embed failed: %v", err)
		}
	}
}

// TestEmbedLatency tests that embedding latency is under 10ms.
func TestEmbedLatency(t *testing.T) {
	// Skip if ONNX runtime is not available
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
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	if err := engine.Initialize(sharedLibPath); err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}
	defer engine.Shutdown()

	testTexts := []string{
		"Write a Python function to calculate the factorial of a number",
		"Explain the concept of machine learning",
		"How do I fix a null pointer exception in Java?",
		"Create a REST API endpoint for user authentication",
		"What is the difference between TCP and UDP?",
	}

	// Warm up
	for _, text := range testTexts {
		engine.Embed(text)
	}

	// Measure latency
	var totalLatency time.Duration
	iterations := 10

	for i := 0; i < iterations; i++ {
		for _, text := range testTexts {
			start := time.Now()
			_, err := engine.Embed(text)
			elapsed := time.Since(start)
			
			if err != nil {
				t.Fatalf("Embed failed: %v", err)
			}
			
			totalLatency += elapsed
		}
	}

	avgLatency := totalLatency / time.Duration(iterations*len(testTexts))
	t.Logf("Average embedding latency: %v", avgLatency)

	// Requirement: <10ms latency
	if avgLatency > 10*time.Millisecond {
		t.Errorf("Average latency %v exceeds 10ms requirement", avgLatency)
	}
}

// BenchmarkBatchEmbed benchmarks batch embedding performance.
func BenchmarkBatchEmbed(b *testing.B) {
	// Skip if ONNX runtime is not available
	locator := NewModelLocator()
	if !locator.ModelExists(DefaultModelName) {
		b.Skip("Embedding model not available. Run scripts/download-embedding-model.sh to download.")
	}

	sharedLibPath := locator.GetSharedLibraryPath()
	if sharedLibPath == "" {
		b.Skip("ONNX runtime shared library not found. Install onnxruntime first.")
	}

	cfg := Config{
		ModelPath:         locator.GetModelPath(DefaultModelName),
		VocabPath:         locator.GetVocabPath(DefaultModelName),
		SharedLibraryPath: sharedLibPath,
	}

	engine, err := NewEngine(cfg)
	if err != nil {
		b.Fatalf("Failed to create engine: %v", err)
	}

	if err := engine.Initialize(sharedLibPath); err != nil {
		b.Fatalf("Failed to initialize engine: %v", err)
	}
	defer engine.Shutdown()

	testTexts := []string{
		"Write a Python function to calculate the factorial of a number",
		"Explain the concept of machine learning",
		"How do I fix a null pointer exception in Java?",
		"Create a REST API endpoint for user authentication",
		"What is the difference between TCP and UDP?",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.BatchEmbed(testTexts)
		if err != nil {
			b.Fatalf("BatchEmbed failed: %v", err)
		}
	}
}

// BenchmarkCosineSimilarity benchmarks cosine similarity computation.
func BenchmarkCosineSimilarity(b *testing.B) {
	engine := &Engine{dimension: EmbeddingDimension}

	// Create two random-ish vectors
	a := make([]float32, EmbeddingDimension)
	bVec := make([]float32, EmbeddingDimension)
	for i := 0; i < EmbeddingDimension; i++ {
		a[i] = float32(i) / float32(EmbeddingDimension)
		bVec[i] = float32(EmbeddingDimension-i) / float32(EmbeddingDimension)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.CosineSimilarity(a, bVec)
	}
}
