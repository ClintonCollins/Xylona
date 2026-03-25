package alerts

import (
	"encoding/json"
	"fmt"
	"math"
)

const thresholdFloatEqualityEpsilon = 0.000001

// ThresholdEventData is the shared JSON payload for threshold-based alert
// events across evaluator, delivery, and federation forwarding.
type ThresholdEventData struct {
	CurrentValue float64 `json:"current_value"`
	Threshold    float64 `json:"threshold"`
	Direction    string  `json:"direction,omitempty"`
}

func thresholdValuesEqual(left, right float64) bool {
	return math.Abs(left-right) <= thresholdFloatEqualityEpsilon
}

// conditionPayload is the JSON structure stored in rule condition fields.
type conditionPayload struct {
	Operator string  `json:"operator"`
	Value    float64 `json:"value"`
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

// ParseConditionJSON extracts the operator and threshold value from a rule's
// JSON condition string. Returns an error if the JSON is empty, malformed, or
// missing the operator field.
func ParseConditionJSON(conditionJSON string) (operator string, threshold float64, err error) {
	if conditionJSON == "" {
		return "", 0, fmt.Errorf("condition JSON is empty")
	}
	var cond conditionPayload
	errUnmarshal := json.Unmarshal([]byte(conditionJSON), &cond)
	if errUnmarshal != nil {
		return "", 0, fmt.Errorf("invalid condition JSON: %w", errUnmarshal)
	}
	if cond.Operator == "" {
		return "", 0, fmt.Errorf("condition has no operator")
	}
	return cond.Operator, cond.Value, nil
}
