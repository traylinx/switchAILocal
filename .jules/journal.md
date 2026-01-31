## 2025-05-24 - SSE Streaming Allocations
**Learning:** Using `fmt.Sprintf` and intermediate string conversions for constructing Server-Sent Events (SSE) messages generates significant allocation overhead in hot paths. Specifically, converting `[]byte` from `json.Marshal` to `string` just to format it into another string is wasteful.
**Action:** Use `strings.Builder` to construct SSE messages, writing the byte slice from `json.Marshal` directly to the builder. This avoids multiple string allocations and copying.
