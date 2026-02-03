## 2025-05-24 - SSE Streaming Allocations
**Learning:** Using `fmt.Sprintf` and intermediate string conversions for constructing Server-Sent Events (SSE) messages generates significant allocation overhead in hot paths. Specifically, converting `[]byte` from `json.Marshal` to `string` just to format it into another string is wasteful.
**Action:** Use `strings.Builder` to construct SSE messages, writing the byte slice from `json.Marshal` directly to the builder. This avoids multiple string allocations and copying.

## 2025-05-27 - Direct Writer for SSE Streaming
**Learning:** Even `fmt.Fprintf` allocates significantly when used with `%s` and string conversions in hot loops (streaming chunks).
**Action:** Use `http.ResponseWriter.Write` directly with pre-allocated byte slice constants for static parts (like "data: " and "\n\n") to eliminate allocations and formatted I/O overhead. This reduced allocations from 2/op to 0/op and latency from ~320ns to ~8ns in microbenchmarks.
