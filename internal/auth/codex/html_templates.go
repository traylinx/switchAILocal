// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package codex

import (
	_ "embed"
)

//go:embed templates/login_success.html
var LoginSuccessHtml string

//go:embed templates/setup_notice.html
var SetupNoticeHtml string
