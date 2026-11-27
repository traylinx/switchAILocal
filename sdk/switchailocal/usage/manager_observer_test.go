// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package usage

import (
	"context"
	"errors"
	"testing"
)

type recordingObserver struct {
	called bool
	tokens int64
}

func TestRequiredModelAuthorizationMarkerSurvivesContextPropagation(t *testing.T) {
	source := WithModelAuthorizationRequired(context.Background())
	propagated := PropagateSynchronousObserver(context.Background(), source)
	if err := AuthorizeModel(propagated, "model-a"); !errors.Is(err, ErrModelAuthorizerUnavailable) {
		t.Fatalf("authorization error = %v, want %v", err, ErrModelAuthorizerUnavailable)
	}
}

func (o *recordingObserver) ObserveUsage(_ context.Context, record Record) {
	o.called = true
	o.tokens = record.Detail.TotalTokens
}

func TestPublishRecordInvokesRequestObserverSynchronously(t *testing.T) {
	observer := &recordingObserver{}
	ctx := WithSynchronousObserver(context.Background(), observer)
	PublishRecord(ctx, Record{Detail: Detail{TotalTokens: 7}})
	if !observer.called || observer.tokens != 7 {
		t.Fatalf("observer was not called synchronously: %#v", observer)
	}
}
