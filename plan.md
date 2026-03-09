1. **Understand the problem**:
   - The memory context mentions: "The non-streaming Claude response converters (`ConvertClaudeResponseToOpenAIResponsesNonStream` and `ConvertClaudeResponseToGeminiNonStream`) use a manual byte loop with `bytes.IndexByte` instead of `bufio.Scanner` to eliminate a 50MB buffer allocation per request."
   - However, `ConvertClaudeResponseToOpenAIResponsesNonStream` in `internal/translator/claude/openai/responses/claude_openai-responses_response.go` and `ConvertAnthropicResponseToGeminiNonStream` in `internal/translator/claude/gemini/claude_gemini_response.go` currently use `bufio.Scanner` with a massive 50MB buffer `make([]byte, 52_428_800)` per request. This allocation is extremely expensive for high-volume APIs.
   - We need to replace the `bufio.Scanner` approach with a manual byte loop using `bytes.IndexByte` in these two files.

2. **Files to modify**:
   - `internal/translator/claude/openai/responses/claude_openai-responses_response.go`
   - `internal/translator/claude/gemini/claude_gemini_response.go`

3. **Approach**:
   - For `ConvertClaudeResponseToOpenAIResponsesNonStream`:
     ```go
     // Replace this:
     var chunks [][]byte
     {
         scanner := bufio.NewScanner(bytes.NewReader(rawJSON))
         buf := make([]byte, 52_428_800) // 50MB
         scanner.Buffer(buf, 52_428_800)
         for scanner.Scan() {
             line := scanner.Bytes()
             if !bytes.HasPrefix(line, dataTag) {
                 continue
             }
             chunks = append(chunks, line[len(dataTag):])
         }
     }

     // With this:
     var chunks [][]byte
     {
         data := rawJSON
         for len(data) > 0 {
             idx := bytes.IndexByte(data, '\n')
             var line []byte
             if idx >= 0 {
                 line = data[:idx]
                 data = data[idx+1:]
             } else {
                 line = data
                 data = nil
             }

             // Optional: handle carriage returns if present
             if len(line) > 0 && line[len(line)-1] == '\r' {
                 line = line[:len(line)-1]
             }

             if bytes.HasPrefix(line, dataTag) {
                 chunks = append(chunks, line[len(dataTag):])
             }
         }
     }
     ```

   - For `ConvertAnthropicResponseToGeminiNonStream` (Wait, let me double check the function name in `claude_gemini_response.go`):
     ```go
     // Let's inspect the code around line 293 in claude_gemini_response.go
     ```
