package alerts

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

const thresholdFloatEqualityEpsilon = 0.000001

// ThresholdEventData is the shared JSON payload for threshold-based alert
// events across evaluator and delivery paths.
type ThresholdEventData struct {
	CurrentValue float64 `json:"current_value"`
	Threshold    float64 `json:"threshold"`
	Direction    string  `json:"direction,omitempty"`
}

func thresholdValuesEqual(left, right float64) bool {
	return math.Abs(left-right) <= thresholdFloatEqualityEpsilon
}

// ThresholdCondition is the versioned behavior encoded in a threshold rule's
// condition JSON. Zero-valued timing fields preserve the legacy immediate,
// transition-only behavior.
type ThresholdCondition struct {
	Operator        string   `json:"operator"`
	Value           float64  `json:"value"`
	ForSeconds      int64    `json:"for_seconds,omitempty"`
	RecoveryValue   *float64 `json:"recovery_value,omitempty"`
	CooldownSeconds int64    `json:"cooldown_seconds,omitempty"`
	RepeatSeconds   int64    `json:"repeat_seconds,omitempty"`
	NoDataSeconds   int64    `json:"no_data_seconds,omitempty"`
}

// EvaluateThresholdOp compares actual against threshold using the given
// operator string. Supported operators: ">=", ">", "<=", "<", "==", "=".
func EvaluateThresholdOp(op string, threshold, actual float64) (bool, error) {
	switch op {
	case ">=":
		return actual >= threshold, nil
	case ">":
		return actual > threshold, nil
	case "<=":
		return actual <= threshold, nil
	case "<":
		return actual < threshold, nil
	case "==", "=":
		return thresholdValuesEqual(actual, threshold), nil
	default:
		return false, fmt.Errorf("unknown threshold operator: %q", op)
	}
}

// ParseThresholdConditionJSON parses and validates all threshold behavior
// fields while remaining backward-compatible with operator/value-only rules.
func ParseThresholdConditionJSON(conditionJSON string) (ThresholdCondition, error) {
	if conditionJSON == "" {
		return ThresholdCondition{}, errors.New("condition JSON is empty")
	}
	var cond ThresholdCondition
	errUnmarshal := json.Unmarshal([]byte(conditionJSON), &cond)
	if errUnmarshal != nil {
		return ThresholdCondition{}, fmt.Errorf("invalid condition JSON: %w", errUnmarshal)
	}
	if cond.Operator == "" {
		return ThresholdCondition{}, errors.New("condition has no operator")
	}
	_, errOperator := EvaluateThresholdOp(cond.Operator, cond.Value, cond.Value)
	if errOperator != nil {
		return ThresholdCondition{}, errOperator
	}
	if math.IsNaN(cond.Value) || math.IsInf(cond.Value, 0) {
		return ThresholdCondition{}, errors.New("condition value must be finite")
	}
	if cond.RecoveryValue != nil && (math.IsNaN(*cond.RecoveryValue) || math.IsInf(*cond.RecoveryValue, 0)) {
		return ThresholdCondition{}, errors.New("condition recovery value must be finite")
	}
	if cond.RecoveryValue != nil {
		switch cond.Operator {
		case ">=", ">":
			if *cond.RecoveryValue >= cond.Value {
				return ThresholdCondition{}, errors.New("condition recovery value must be below the trigger value")
			}
		case "<=", "<":
			if *cond.RecoveryValue <= cond.Value {
				return ThresholdCondition{}, errors.New("condition recovery value must be above the trigger value")
			}
		case "==", "=":
			return ThresholdCondition{}, errors.New("equality conditions cannot use a recovery value")
		}
	}
	if cond.ForSeconds < 0 || cond.CooldownSeconds < 0 || cond.RepeatSeconds < 0 || cond.NoDataSeconds < 0 {
		return ThresholdCondition{}, errors.New("condition durations cannot be negative")
	}
	return cond, nil
}
