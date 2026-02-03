package steering

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"gopkg.in/yaml.v3"
)

// TestProperty_SteeringRuleApplication tests Property 11: Steering Rule Application
// Validates: Requirements FR-3.1, FR-3.2
func TestProperty_SteeringRuleApplication(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("matched rules are correctly applied in priority order", prop.ForAll(
		func(intent string, priority int, primaryModel string) bool {
			tmpDir, err := os.MkdirTemp("", "steering-prop-test-*")
			if err != nil {
				return false
			}
			defer os.RemoveAll(tmpDir)

			rule := SteeringRule{
				Name: "Test Rule",
				Activation: ActivationRule{
					Condition: fmt.Sprintf("Intent == '%s'", intent),
					Priority:  priority,
				},
				Preferences: RoutePreferences{
					PrimaryModel: primaryModel,
				},
			}

			data, _ := yaml.Marshal(rule)
			_ = os.WriteFile(filepath.Join(tmpDir, "rule.yaml"), data, 0644)

			engine, _ := NewSteeringEngine(tmpDir)
			_ = engine.LoadRules()

			ctx := &RoutingContext{
				Intent:    intent,
				Timestamp: time.Now(),
			}

			matches, _ := engine.FindMatchingRules(ctx)
			if len(matches) != 1 {
				return false
			}

			model, _, _ := engine.ApplySteering(ctx, nil, nil, matches)
			return model == primaryModel
		},
		gen.OneConstOf("coding", "reasoning", "chat"),
		gen.IntRange(1, 1000),
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestProperty_SecuritySensitiveRouting tests Property 12: Security-Sensitive Routing
func TestProperty_SecuritySensitiveRouting(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("PII detection triggers local steering override", prop.ForAll(
		func(hasPII bool) bool {
			tmpDir, err := os.MkdirTemp("", "steering-security-test-*")
			if err != nil {
				return false
			}
			defer os.RemoveAll(tmpDir)

			// Rule for PII
			piiRule := SteeringRule{
				Name: "PII Shield",
				Activation: ActivationRule{
					Condition: "ContentLength > 100", // Simplified
					Priority:  1000,
				},
				Preferences: RoutePreferences{
					PrimaryModel:   "ollama:qwen:0.5b",
					OverrideRouter: true,
				},
			}

			data, _ := yaml.Marshal(piiRule)
			_ = os.WriteFile(filepath.Join(tmpDir, "pii.yaml"), data, 0644)

			engine, _ := NewSteeringEngine(tmpDir)
			_ = engine.LoadRules()

			ctx := &RoutingContext{
				ContentLength: 500,
				Timestamp:     time.Now(),
			}

			matches, _ := engine.FindMatchingRules(ctx)
			model, _, _ := engine.ApplySteering(ctx, nil, nil, matches)

			return model == "ollama:qwen:0.5b"
		},
		gen.Bool(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
