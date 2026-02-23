## 2026-02-23 - Massive Buffer Allocation in Non-Stream Handler
**Learning:** Found a 50MB per-request buffer allocation in `ConvertClaudeResponseToOpenAIResponsesNonStream` due to `bufio.Scanner` default buffer size handling, which was intended to handle large lines but allocated on every call regardless of content size. This caused massive memory pressure under load.
**Action:** Avoid `bufio.Scanner` for simple line splitting when you have the full byte slice. Use `bytes.IndexByte` or similar for zero-allocation parsing.
