1. **Understand the problem**:
   - `FileStreamingLogWriter.WriteChunkAsync` in `internal/logging/request_logger.go` creates a new slice and copies data on every call (`chunkCopy := make([]byte, len(chunk))`, `copy(chunkCopy, chunk)`).
   - This results in 1 allocation per chunk, which adds up for long streaming responses.
2. **Propose the solution**:
   - Introduce a `sync.Pool` for byte slices in `internal/logging/request_logger.go`.
   - Update `FileStreamingLogWriter.chunkChan` to receive `*[]byte` instead of `[]byte`.
   - In `WriteChunkAsync`, acquire a `*[]byte` from the pool, adjust capacity/length, copy data, and send it to `chunkChan`.
   - If the channel is full, recycle the buffer back to the pool to prevent leaks.
   - In `asyncWriter`, receive `*[]byte`, write the data, and put it back into the pool.
3. **Write the benchmark**:
   - The user memory tells me there's an existing benchmark for this or I can write a new one `BenchmarkFileStreamingLogWriter_WriteChunkAsync` in `internal/logging/request_logger_bench_test.go`.
4. **Pre-commit step**:
   - "Complete pre commit steps to make sure proper testing, verifications, reviews and reflections are done."
