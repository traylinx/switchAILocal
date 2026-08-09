// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"testing"
	"time"

	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// A cancelled context (e.g. client disconnect) must unblock a send to a channel
// nobody is reading — this is what stops the streaming goroutine/connection leak.
func TestSendStreamChunk_UnblocksOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := make(chan switchailocalexecutor.StreamChunk) // unbuffered, no reader
	done := make(chan bool, 1)
	go func() {
		done <- sendStreamChunk(ctx, out, switchailocalexecutor.StreamChunk{Payload: []byte("x")})
	}()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("sendStreamChunk returned true on a cancelled context; expected false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sendStreamChunk blocked on a cancelled context with no reader — leak not fixed")
	}
}

// When a reader is present and the context is live, the chunk must be delivered
// and the call must report success.
func TestSendStreamChunk_DeliversWhenReaderPresent(t *testing.T) {
	out := make(chan switchailocalexecutor.StreamChunk, 1)

	ok := sendStreamChunk(context.Background(), out, switchailocalexecutor.StreamChunk{Payload: []byte("hello")})
	if !ok {
		t.Fatal("sendStreamChunk returned false despite a ready buffered channel")
	}
	got := <-out
	if string(got.Payload) != "hello" {
		t.Fatalf("delivered payload = %q, want %q", got.Payload, "hello")
	}
}

// End-to-end model of the leak-and-fix: a producer streams onto an unbuffered
// channel, the consumer reads one chunk then stops (client disconnect), and the
// context is cancelled. The producer must observe the cancellation on its next
// send, return, and run its deferred cleanup (which in the real executors closes
// the upstream response body and stops the watchdog). With the previous bare
// `out <- chunk`, the producer would block forever here and the cleanup would
// never run — the goroutine + connection leak this test guards against.
func TestSendStreamChunk_ProducerExitsAndCleansUpOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan switchailocalexecutor.StreamChunk) // unbuffered; consumer stops reading
	cleaned := make(chan struct{})

	go func() {
		defer close(cleaned) // stands in for httpResp.Body.Close() / watchdog.stop()
		defer close(out)
		for {
			if !sendStreamChunk(ctx, out, switchailocalexecutor.StreamChunk{Payload: []byte("x")}) {
				return
			}
		}
	}()

	<-out    // consume exactly one chunk, then stop reading
	cancel() // simulate the client going away

	select {
	case <-cleaned:
		// producer observed the cancellation, returned, and ran deferred cleanup
	case <-time.After(2 * time.Second):
		t.Fatal("producer blocked on send after cancel with no reader — deferred cleanup never ran (leak)")
	}
}
