// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"

	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// sendStreamChunk delivers chunk on out, aborting if ctx is cancelled.
//
// Streaming executors push chunks onto an unbuffered channel that a relay/handler
// reads. When the client disconnects mid-stream the consumer stops reading, so a
// bare `out <- chunk` would block the producer goroutine forever — leaking the
// goroutine and the upstream HTTP connection because the deferred Body.Close()
// never runs. Selecting on ctx.Done() lets the producer unwind and clean up.
//
// It returns false when the send was abandoned because ctx was done, signalling
// the caller to stop producing and return so its deferred cleanup runs.
//
// Pass the request context (client-disconnect signal), NOT a watchdog-derived
// child context: a stall watchdog cancels its child to interrupt the upstream
// read, and the terminal stall/error chunk must still reach the failover layer.
// Guarding that send on the already-cancelled child would randomly drop it.
//
// A nil ctx falls back to a plain blocking send, preserving the pre-existing
// behaviour rather than panicking on ctx.Done().
func sendStreamChunk(ctx context.Context, out chan<- switchailocalexecutor.StreamChunk, chunk switchailocalexecutor.StreamChunk) bool {
	if ctx == nil {
		out <- chunk
		return true
	}
	select {
	case out <- chunk:
		return true
	case <-ctx.Done():
		return false
	}
}
