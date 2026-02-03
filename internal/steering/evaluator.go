package steering

import (
	"fmt"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// ConditionEvaluator handles the evaluation of activation conditions.
type ConditionEvaluator struct {
	// Precompiled programs for better performance
	programs map[string]*vm.Program
}

// NewConditionEvaluator creates a new condition evaluator.
func NewConditionEvaluator() *ConditionEvaluator {
	return &ConditionEvaluator{
		programs: make(map[string]*vm.Program),
	}
}

// Evaluate evaluates a condition string against the provided routing context.
func (e *ConditionEvaluator) Evaluate(condition string, ctx *RoutingContext) (bool, error) {
	if condition == "" || condition == "true" {
		return true, nil
	}

	program, exists := e.programs[condition]
	if !exists {
		var err error
		program, err = expr.Compile(condition, expr.Env(ctx))
		if err != nil {
			return false, fmt.Errorf("failed to compile condition '%s': %w", condition, err)
		}
		// In a production environment, we'd use a mutex here if rules change frequently.
		// For now, we'll assume rules are loaded primarily at startup or via controlled reload.
		e.programs[condition] = program
	}

	output, err := expr.Run(program, ctx)
	if err != nil {
		return false, fmt.Errorf("failed to run condition '%s': %w", condition, err)
	}

	result, ok := output.(bool)
	if !ok {
		return false, fmt.Errorf("condition '%s' did not return a boolean", condition)
	}

	return result, nil
}

// CheckTimeRule checks if the current time matches a time-based rule.
func (e *ConditionEvaluator) CheckTimeRule(rule TimeBasedRule, now time.Time) bool {
	// Check hour range
	hour := now.Hour()
	if !e.isInHourRange(hour, rule.Hours) {
		return false
	}

	// Check day of week
	if !e.isInDayRange(now.Weekday(), rule.Days) {
		return false
	}

	return true
}

// isInHourRange checks if the hour is within the specified range.
// Format: "9-17" or "9-11,14-17" or empty (matches all)
func (e *ConditionEvaluator) isInHourRange(hour int, hoursStr string) bool {
	if hoursStr == "" {
		return true // No restriction
	}

	// Parse hour ranges (e.g., "9-17" or "9-11,14-17")
	ranges := strings.Split(hoursStr, ",")
	for _, r := range ranges {
		r = strings.TrimSpace(r)
		parts := strings.Split(r, "-")
		if len(parts) == 2 {
			var start, end int
			_, _ = fmt.Sscanf(parts[0], "%d", &start)
			_, _ = fmt.Sscanf(parts[1], "%d", &end)
			if hour >= start && hour <= end {
				return true
			}
		} else if len(parts) == 1 {
			// Single hour
			var single int
			_, _ = fmt.Sscanf(parts[0], "%d", &single)
			if hour == single {
				return true
			}
		}
	}

	return false
}

// isInDayRange checks if the day of week is within the specified range.
// Format: "Mon-Fri" or "Mon,Wed,Fri" or empty (matches all)
func (e *ConditionEvaluator) isInDayRange(weekday time.Weekday, daysStr string) bool {
	if daysStr == "" {
		return true // No restriction
	}

	dayMap := map[string]time.Weekday{
		"Sun": time.Sunday,
		"Mon": time.Monday,
		"Tue": time.Tuesday,
		"Wed": time.Wednesday,
		"Thu": time.Thursday,
		"Fri": time.Friday,
		"Sat": time.Saturday,
	}

	// Parse day ranges (e.g., "Mon-Fri" or "Mon,Wed,Fri")
	if strings.Contains(daysStr, "-") {
		// Range format: "Mon-Fri"
		parts := strings.Split(daysStr, "-")
		if len(parts) == 2 {
			start := dayMap[strings.TrimSpace(parts[0])]
			end := dayMap[strings.TrimSpace(parts[1])]

			// Check if weekday is in range
			if start <= end {
				return weekday >= start && weekday <= end
			} else {
				// Wrap around (e.g., "Fri-Mon")
				return weekday >= start || weekday <= end
			}
		}
	} else {
		// List format: "Mon,Wed,Fri"
		days := strings.Split(daysStr, ",")
		for _, d := range days {
			if dayMap[strings.TrimSpace(d)] == weekday {
				return true
			}
		}
	}

	return false
}
