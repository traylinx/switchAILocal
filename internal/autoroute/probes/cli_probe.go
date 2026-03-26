package probes

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/traylinx/switchAILocal/internal/autoroute"
)

// CLIProber probes locally installed CLI providers like gemini, claude, codex
type CLIProber struct {
	providerName string // e.g. "geminicli", "claudecli"
	binaryName   string // e.g. "gemini", "claude"
}

// NewCLIProber creates a prober for a specific CLI tool
func NewCLIProber(providerName, binaryName string) *CLIProber {
	return &CLIProber{
		providerName: providerName,
		binaryName:   binaryName,
	}
}

// Name returns the provider identifier
func (p *CLIProber) Name() string {
	return p.providerName
}

// Probe executes a fast CLI command to determine availability and basic info
func (p *CLIProber) Probe(ctx context.Context) autoroute.ProbeResult {
	start := time.Now()
	result := autoroute.ProbeResult{
		Provider: p.providerName,
		AuthType: autoroute.AuthTypeUnknown,
	}

	// 1. Basic availability check (is it installed?)
	cmd := exec.CommandContext(ctx, p.binaryName, "--version")
	_, err := cmd.CombinedOutput()
	result.Latency = time.Since(start)

	if err != nil {
		result.Available = false
		result.ProbeError = err
		return result
	}

	result.Available = true

	// 2. Specific heuristics based on the binary
	switch p.binaryName {
	case "claude":
		// Probe tier (e.g. Free vs Pro vs Team)
		// We'll run a lightweight command that reveals account or tier info if active
		tierCmd := exec.CommandContext(ctx, p.binaryName, "account")
		tierOut, err := tierCmd.CombinedOutput()
		if err == nil {
			tierStr := string(tierOut)
			// Simple heuristic parsing of claude account output
			result.SubscriptionInfo = &autoroute.SubscriptionInfo{
				Tier:   extractClaudeTier(tierStr),
				Source: "cli-command",
			}
			result.AuthType = autoroute.AuthTypeOAuth
		}

	case "gemini":
		// If gemini --version succeeded, guess it's standard OAuth unless specified otherwise
		result.AuthType = autoroute.AuthTypeOAuth
		result.SubscriptionInfo = &autoroute.SubscriptionInfo{
			Tier:   "standard",
			Source: "inferred",
		}

	case "codex":
		result.AuthType = autoroute.AuthTypeOAuth
		result.SubscriptionInfo = &autoroute.SubscriptionInfo{
			Tier:   "standard",
			Source: "inferred",
		}
	}

	return result
}

// extractClaudeTier is a regex/string helper to find the subscription tier
func extractClaudeTier(output string) string {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "pro") {
		return "pro"
	}
	if strings.Contains(lower, "team") {
		return "team"
	}
	if strings.Contains(lower, "enterprise") {
		return "enterprise"
	}
	if strings.Contains(lower, "free") || strings.Contains(lower, "basic") {
		return "free"
	}
	return "unknown"
}
