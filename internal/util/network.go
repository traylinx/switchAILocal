// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"net"

	"github.com/gin-gonic/gin"
)

// IsLocalhostDirect checks if the request is coming directly from localhost
// without any proxy headers, ensuring a secure local connection.
func IsLocalhostDirect(c *gin.Context) bool {
	// 1. Check strict RemoteAddr
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		// If RemoteAddr is invalid (e.g. pipe), assume unsafe for localhost checks
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}

	// 2. Ensure no proxy headers are present to prevent spoofing or proxy bypass
	// If the user is running behind a proxy, they should not use "Direct Localhost" endpoints.
	if c.GetHeader("X-Forwarded-For") != "" ||
		c.GetHeader("X-Real-IP") != "" ||
		c.GetHeader("Forwarded") != "" {
		return false
	}

	return true
}
