// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package config provides the public SDK configuration API.
//
// It re-exports the server configuration types and helpers so external projects can
// embed switchAILocal without importing internal packages.
package config

import internalconfig "github.com/traylinx/switchAILocal/internal/config"

type SDKConfig = internalconfig.SDKConfig
type AccessConfig = internalconfig.AccessConfig
type AccessProvider = internalconfig.AccessProvider

type Config = internalconfig.Config

type StreamingConfig = internalconfig.StreamingConfig
type TLSConfig = internalconfig.TLSConfig
type RemoteManagement = internalconfig.RemoteManagement
type AmpCode = internalconfig.AmpCode
type PayloadConfig = internalconfig.PayloadConfig
type PayloadRule = internalconfig.PayloadRule
type PayloadModelRule = internalconfig.PayloadModelRule
type IntelligenceConfig = internalconfig.IntelligenceConfig

type GeminiKey = internalconfig.GeminiKey
type CodexKey = internalconfig.CodexKey
type ClaudeKey = internalconfig.ClaudeKey
type VertexCompatKey = internalconfig.VertexCompatKey
type VertexCompatModel = internalconfig.VertexCompatModel
type OpenAICompatibility = internalconfig.OpenAICompatibility
type OpenAICompatibilityAPIKey = internalconfig.OpenAICompatibilityAPIKey
type OpenAICompatibilityModel = internalconfig.OpenAICompatibilityModel

type SwitchAIKey = internalconfig.SwitchAIKey

type TLS = internalconfig.TLSConfig

const (
	AccessProviderTypeConfigAPIKey = internalconfig.AccessProviderTypeConfigAPIKey
	DefaultAccessProviderName      = internalconfig.DefaultAccessProviderName
	DefaultPanelGitHubRepository   = internalconfig.DefaultPanelGitHubRepository
)

func MakeInlineAPIKeyProvider(keys []string) *AccessProvider {
	return internalconfig.MakeInlineAPIKeyProvider(keys)
}

func LoadConfig(configFile string) (*Config, error) { return internalconfig.LoadConfig(configFile) }

func LoadConfigOptional(configFile string, optional bool) (*Config, error) {
	return internalconfig.LoadConfigOptional(configFile, optional)
}

func SaveConfigPreserveComments(configFile string, cfg *Config) error {
	return internalconfig.SaveConfigPreserveComments(configFile, cfg)
}

func SaveConfigPreserveCommentsUpdateNestedScalar(configFile string, path []string, value string) error {
	return internalconfig.SaveConfigPreserveCommentsUpdateNestedScalar(configFile, path, value)
}

func NormalizeCommentIndentation(data []byte) []byte {
	return internalconfig.NormalizeCommentIndentation(data)
}
