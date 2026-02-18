package responses

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkFunctionCallAggregation(b *testing.B) {
	// Simulate a scenario with multiple function calls being streamed.
	// We'll simulate 5 function calls, each with some arguments.
	ctx := context.Background()
	model := "gemini-pro"

	// Pre-generate JSON chunks simulating streaming function calls
	var chunks [][]byte

	// 5 function calls
	for i := 0; i < 5; i++ {
		// Initial call
		chunks = append(chunks, []byte(fmt.Sprintf(`{
			"candidates": [{
				"content": {
					"parts": [{
						"functionCall": {
							"name": "func_%d",
							"args": "{"
						}
					}]
				}
			}]
		}`, i)))

		// Args chunks
		for j := 0; j < 5; j++ {
			chunks = append(chunks, []byte(fmt.Sprintf(`{
				"candidates": [{
					"content": {
						"parts": [{
							"functionCall": {
								"name": "func_%d",
								"args": "\"arg_%d\": %d,"
							}
						}]
					}
				}
			}]`, i, j, j)))
		}

		// Closing args
		chunks = append(chunks, []byte(fmt.Sprintf(`{
			"candidates": [{
				"content": {
					"parts": [{
						"functionCall": {
							"name": "func_%d",
							"args": "\"done\": true}"
						}
					}]
				}
			}]
		}`, i)))
	}

	// Final chunk to trigger sorting/finalization
	chunks = append(chunks, []byte(`{
		"candidates": [{
			"finishReason": "STOP"
		}]
	}`))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var param any
		for _, chunk := range chunks {
			_ = ConvertGeminiResponseToOpenAIResponses(ctx, model, nil, nil, chunk, &param)
		}
	}
}
