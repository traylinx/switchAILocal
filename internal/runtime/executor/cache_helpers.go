// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import "time"

type codexCache struct {
	ID     string
	Expire time.Time
}

var codexCacheMap = map[string]codexCache{}
