// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package embedding

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// TokenizedInput represents the tokenized output ready for model inference.
type TokenizedInput struct {
	// InputIDs are the token IDs
	InputIDs []int64

	// AttentionMask indicates which tokens are real (1) vs padding (0)
	AttentionMask []int64

	// TokenTypeIDs are segment IDs (0 for first segment)
	TokenTypeIDs []int64
}

// SimpleTokenizer implements a basic WordPiece tokenizer for BERT-style models.
// This is a simplified implementation that handles common cases.
type SimpleTokenizer struct {
	// vocab maps tokens to their IDs
	vocab map[string]int64

	// idToToken maps IDs back to tokens
	idToToken map[int64]string

	// Special token IDs
	clsTokenID int64
	sepTokenID int64
	padTokenID int64
	unkTokenID int64
}

// NewSimpleTokenizer creates a new tokenizer from a vocabulary file.
// The vocabulary file should have one token per line.
//
// Parameters:
//   - vocabPath: Path to the vocabulary file
//
// Returns:
//   - *SimpleTokenizer: A new tokenizer instance
//   - error: Any error encountered during loading
func NewSimpleTokenizer(vocabPath string) (*SimpleTokenizer, error) {
	t := &SimpleTokenizer{
		vocab:     make(map[string]int64),
		idToToken: make(map[int64]string),
	}

	// If no vocab path provided, use built-in minimal vocab
	if vocabPath == "" {
		t.initMinimalVocab()
		return t, nil
	}

	// Load vocabulary from file
	file, err := os.Open(vocabPath)
	if err != nil {
		// Fall back to minimal vocab if file not found
		t.initMinimalVocab()
		return t, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var id int64
	for scanner.Scan() {
		token := scanner.Text()
		t.vocab[token] = id
		t.idToToken[id] = token
		id++
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading vocabulary: %w", err)
	}

	// Set special token IDs
	t.setSpecialTokenIDs()

	return t, nil
}

// initMinimalVocab initializes a minimal vocabulary for basic operation.
// This allows the tokenizer to work without a vocabulary file.
func (t *SimpleTokenizer) initMinimalVocab() {
	// Minimal BERT vocabulary with special tokens and common words
	minimalVocab := []string{
		"[PAD]", "[UNK]", "[CLS]", "[SEP]", "[MASK]",
		"the", "a", "an", "is", "are", "was", "were",
		"to", "of", "in", "for", "on", "with", "at",
		"by", "from", "as", "or", "and", "but", "not",
		"this", "that", "it", "be", "have", "has", "had",
		"do", "does", "did", "will", "would", "could", "should",
		"can", "may", "might", "must", "shall",
		"i", "you", "he", "she", "we", "they", "me", "him", "her", "us", "them",
		"my", "your", "his", "its", "our", "their",
		"what", "which", "who", "whom", "whose", "where", "when", "why", "how",
		"all", "each", "every", "both", "few", "more", "most", "other", "some", "such",
		"no", "nor", "only", "own", "same", "so", "than", "too", "very",
		"just", "also", "now", "here", "there", "then", "once",
		"code", "coding", "program", "programming", "software", "developer",
		"write", "create", "build", "make", "help", "explain", "analyze",
		"data", "file", "function", "class", "method", "variable",
		"error", "bug", "fix", "debug", "test", "testing",
		"api", "web", "server", "client", "database", "query",
		"python", "java", "javascript", "go", "rust", "c", "cpp",
		"##s", "##ed", "##ing", "##er", "##ly", "##tion", "##ment",
	}

	for i, token := range minimalVocab {
		t.vocab[token] = int64(i)
		t.idToToken[int64(i)] = token
	}

	t.setSpecialTokenIDs()
}

// setSpecialTokenIDs sets the IDs for special tokens.
func (t *SimpleTokenizer) setSpecialTokenIDs() {
	if id, ok := t.vocab["[CLS]"]; ok {
		t.clsTokenID = id
	}
	if id, ok := t.vocab["[SEP]"]; ok {
		t.sepTokenID = id
	}
	if id, ok := t.vocab["[PAD]"]; ok {
		t.padTokenID = id
	}
	if id, ok := t.vocab["[UNK]"]; ok {
		t.unkTokenID = id
	}
}

// Tokenize converts text into token IDs for model input.
// It applies basic preprocessing and WordPiece tokenization.
//
// Parameters:
//   - text: The input text to tokenize
//   - maxLength: Maximum sequence length (including special tokens)
//
// Returns:
//   - *TokenizedInput: The tokenized output
//   - error: Any error encountered during tokenization
func (t *SimpleTokenizer) Tokenize(text string, maxLength int) (*TokenizedInput, error) {
	// Preprocess: lowercase and normalize
	text = strings.ToLower(text)
	text = t.normalizeText(text)

	// Split into words
	words := t.splitIntoWords(text)

	// Apply WordPiece tokenization
	tokens := []int64{t.clsTokenID}
	for _, word := range words {
		wordTokens := t.tokenizeWord(word)
		tokens = append(tokens, wordTokens...)

		// Check if we're approaching max length
		if len(tokens) >= maxLength-1 {
			break
		}
	}
	tokens = append(tokens, t.sepTokenID)

	// Truncate if necessary
	if len(tokens) > maxLength {
		tokens = tokens[:maxLength-1]
		tokens = append(tokens, t.sepTokenID)
	}

	// Create attention mask and token type IDs
	seqLen := len(tokens)
	attentionMask := make([]int64, seqLen)
	tokenTypeIDs := make([]int64, seqLen)

	for i := 0; i < seqLen; i++ {
		attentionMask[i] = 1
		tokenTypeIDs[i] = 0
	}

	return &TokenizedInput{
		InputIDs:      tokens,
		AttentionMask: attentionMask,
		TokenTypeIDs:  tokenTypeIDs,
	}, nil
}

// normalizeText applies basic text normalization.
func (t *SimpleTokenizer) normalizeText(text string) string {
	// Remove extra whitespace
	text = strings.Join(strings.Fields(text), " ")

	// Basic punctuation handling
	var result strings.Builder
	for _, r := range text {
		if unicode.IsPunct(r) {
			result.WriteRune(' ')
			result.WriteRune(r)
			result.WriteRune(' ')
		} else {
			result.WriteRune(r)
		}
	}

	return strings.Join(strings.Fields(result.String()), " ")
}

// splitIntoWords splits text into words.
func (t *SimpleTokenizer) splitIntoWords(text string) []string {
	return strings.Fields(text)
}

// tokenizeWord applies WordPiece tokenization to a single word.
func (t *SimpleTokenizer) tokenizeWord(word string) []int64 {
	// Check if the whole word is in vocabulary
	if id, ok := t.vocab[word]; ok {
		return []int64{id}
	}

	// Apply WordPiece: try to find longest matching subwords
	tokens := []int64{}
	start := 0

	for start < len(word) {
		end := len(word)
		found := false

		for end > start {
			substr := word[start:end]
			if start > 0 {
				substr = "##" + substr
			}

			if id, ok := t.vocab[substr]; ok {
				tokens = append(tokens, id)
				found = true
				break
			}
			end--
		}

		if !found {
			// Character not found, use UNK
			tokens = append(tokens, t.unkTokenID)
			start++
		} else {
			if start > 0 {
				start = end
			} else {
				start = end
			}
		}
	}

	if len(tokens) == 0 {
		return []int64{t.unkTokenID}
	}

	return tokens
}

// GetVocabSize returns the size of the vocabulary.
func (t *SimpleTokenizer) GetVocabSize() int {
	return len(t.vocab)
}
