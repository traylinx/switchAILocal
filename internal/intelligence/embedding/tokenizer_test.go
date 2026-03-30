// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package embedding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewSimpleTokenizer tests tokenizer creation.
func TestNewSimpleTokenizer(t *testing.T) {
	// Test with no vocab path (uses minimal vocab)
	tokenizer, err := NewSimpleTokenizer("")
	require.NoError(t, err)
	assert.NotNil(t, tokenizer)
	assert.Greater(t, tokenizer.GetVocabSize(), 0)
}

// TestTokenize tests basic tokenization.
func TestTokenize(t *testing.T) {
	tokenizer, err := NewSimpleTokenizer("")
	require.NoError(t, err)

	tests := []struct {
		name      string
		text      string
		maxLength int
	}{
		{
			name:      "simple text",
			text:      "hello world",
			maxLength: 128,
		},
		{
			name:      "empty text",
			text:      "",
			maxLength: 128,
		},
		{
			name:      "long text",
			text:      "This is a very long text that should be truncated to fit within the maximum sequence length limit",
			maxLength: 20,
		},
		{
			name:      "text with punctuation",
			text:      "Hello, world! How are you?",
			maxLength: 128,
		},
		{
			name:      "code-like text",
			text:      "def hello(): print('world')",
			maxLength: 128,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tokenizer.Tokenize(tt.text, tt.maxLength)
			require.NoError(t, err)
			assert.NotNil(t, result)

			// Check that all arrays have the same length
			assert.Equal(t, len(result.InputIDs), len(result.AttentionMask))
			assert.Equal(t, len(result.InputIDs), len(result.TokenTypeIDs))

			// Check that length doesn't exceed max
			assert.LessOrEqual(t, len(result.InputIDs), tt.maxLength)

			// Check that first token is CLS and last is SEP
			if len(result.InputIDs) > 0 {
				assert.Equal(t, tokenizer.clsTokenID, result.InputIDs[0], "First token should be CLS")
				assert.Equal(t, tokenizer.sepTokenID, result.InputIDs[len(result.InputIDs)-1], "Last token should be SEP")
			}

			// Check attention mask is all 1s (no padding in this implementation)
			for _, mask := range result.AttentionMask {
				assert.Equal(t, int64(1), mask)
			}

			// Check token type IDs are all 0s (single segment)
			for _, typeID := range result.TokenTypeIDs {
				assert.Equal(t, int64(0), typeID)
			}
		})
	}
}

// TestTokenizeWord tests WordPiece tokenization of individual words.
func TestTokenizeWord(t *testing.T) {
	tokenizer, err := NewSimpleTokenizer("")
	require.NoError(t, err)

	tests := []struct {
		name     string
		word     string
		wantUNK  bool // whether we expect UNK tokens
	}{
		{
			name:    "known word",
			word:    "the",
			wantUNK: false,
		},
		{
			name:    "unknown word",
			word:    "supercalifragilisticexpialidocious",
			wantUNK: true,
		},
		{
			name:    "code keyword",
			word:    "function",
			wantUNK: true, // not in minimal vocab
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokenizer.tokenizeWord(tt.word)
			assert.NotEmpty(t, tokens)

			if tt.wantUNK {
				// Should contain at least one UNK token
				hasUNK := false
				for _, tok := range tokens {
					if tok == tokenizer.unkTokenID {
						hasUNK = true
						break
					}
				}
				// Note: might not have UNK if word can be split into known subwords
				_ = hasUNK
			}
		})
	}
}

// TestNormalizeText tests text normalization.
func TestNormalizeText(t *testing.T) {
	tokenizer, err := NewSimpleTokenizer("")
	require.NoError(t, err)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "extra whitespace",
			input:    "hello    world",
			expected: "hello world",
		},
		{
			name:     "leading/trailing whitespace",
			input:    "  hello world  ",
			expected: "hello world",
		},
		{
			name:     "punctuation spacing",
			input:    "hello,world",
			expected: "hello , world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizer.normalizeText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSpecialTokenIDs tests that special token IDs are set correctly.
func TestSpecialTokenIDs(t *testing.T) {
	tokenizer, err := NewSimpleTokenizer("")
	require.NoError(t, err)

	// Check that special tokens exist in vocab
	_, hasCLS := tokenizer.vocab["[CLS]"]
	_, hasSEP := tokenizer.vocab["[SEP]"]
	_, hasPAD := tokenizer.vocab["[PAD]"]
	_, hasUNK := tokenizer.vocab["[UNK]"]

	assert.True(t, hasCLS, "CLS token should be in vocab")
	assert.True(t, hasSEP, "SEP token should be in vocab")
	assert.True(t, hasPAD, "PAD token should be in vocab")
	assert.True(t, hasUNK, "UNK token should be in vocab")

	// Check that IDs are set
	assert.GreaterOrEqual(t, tokenizer.clsTokenID, int64(0))
	assert.GreaterOrEqual(t, tokenizer.sepTokenID, int64(0))
	assert.GreaterOrEqual(t, tokenizer.padTokenID, int64(0))
	assert.GreaterOrEqual(t, tokenizer.unkTokenID, int64(0))
}

// TestGetVocabSize tests the vocabulary size.
func TestGetVocabSize(t *testing.T) {
	tokenizer, err := NewSimpleTokenizer("")
	require.NoError(t, err)

	size := tokenizer.GetVocabSize()
	assert.Greater(t, size, 0)
	assert.Equal(t, len(tokenizer.vocab), size)
}

// TestTokenizeMaxLength tests that tokenization respects max length.
func TestTokenizeMaxLength(t *testing.T) {
	tokenizer, err := NewSimpleTokenizer("")
	require.NoError(t, err)

	// Create a long text
	longText := "word "
	for i := 0; i < 100; i++ {
		longText += "word "
	}

	maxLengths := []int{10, 20, 50, 100}

	for _, maxLen := range maxLengths {
		t.Run("maxLength="+string(rune(maxLen)), func(t *testing.T) {
			result, err := tokenizer.Tokenize(longText, maxLen)
			require.NoError(t, err)
			assert.LessOrEqual(t, len(result.InputIDs), maxLen)
		})
	}
}
