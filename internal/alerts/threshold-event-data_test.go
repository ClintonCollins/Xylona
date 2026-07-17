package alerts

import (
	"fmt"
	"testing"
)

func TestEvaluateThresholdOp(t *testing.T) {
	tests := []struct {
		op        string
		threshold float64
		actual    float64
		want      bool
		wantErr   bool
	}{
		{op: ">=", threshold: 80, actual: 90, want: true},
		{op: ">=", threshold: 80, actual: 80, want: true},
		{op: ">=", threshold: 80, actual: 70, want: false},
		{op: ">", threshold: 80, actual: 81, want: true},
		{op: ">", threshold: 80, actual: 80, want: false},
		{op: "<=", threshold: 80, actual: 70, want: true},
		{op: "<=", threshold: 80, actual: 80, want: true},
		{op: "<=", threshold: 80, actual: 90, want: false},
		{op: "<", threshold: 80, actual: 79, want: true},
		{op: "<", threshold: 80, actual: 80, want: false},
		{op: "==", threshold: 42, actual: 42, want: true},
		{op: "==", threshold: 42, actual: 42.0000004, want: true},
		{op: "==", threshold: 42, actual: 43, want: false},
		{op: "=", threshold: 42, actual: 42, want: true},
		{op: "=", threshold: 42, actual: 41.9999996, want: true},
		{op: "=", threshold: 42, actual: 43, want: false},
		{op: "!=", threshold: 0, actual: 0, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.op+"_"+fmt.Sprintf("%.0f_vs_%.0f", tc.actual, tc.threshold), func(t *testing.T) {
			got, errEval := EvaluateThresholdOp(tc.op, tc.threshold, tc.actual)
			if tc.wantErr {
				if errEval == nil {
					t.Fatalf("expected error for op %q, got nil", tc.op)
				}
				return
			}
			if errEval != nil {
				t.Fatalf("unexpected error: %v", errEval)
			}
			if got != tc.want {
				t.Errorf("EvaluateThresholdOp(%q, %v, %v) = %v, want %v",
					tc.op, tc.threshold, tc.actual, got, tc.want)
			}
		})
	}
}

func TestParseConditionJSON(t *testing.T) {
	tests := []struct {
		name          string
		conditionJSON string
		wantOp        string
		wantThreshold float64
		wantErr       bool
	}{
		{
			name:          "valid >= condition",
			conditionJSON: `{"operator":">=","value":80}`,
			wantOp:        ">=",
			wantThreshold: 80,
		},
		{
			name:          "valid < condition",
			conditionJSON: `{"operator":"<","value":10.5}`,
			wantOp:        "<",
			wantThreshold: 10.5,
		},
		{
			name:          "valid == condition",
			conditionJSON: `{"operator":"==","value":0}`,
			wantOp:        "==",
			wantThreshold: 0,
		},
		{
			name:          "invalid JSON",
			conditionJSON: `{bad json`,
			wantErr:       true,
		},
		{
			name:          "missing operator",
			conditionJSON: `{"value":80}`,
			wantErr:       true,
		},
		{
			name:          "empty string",
			conditionJSON: "",
			wantErr:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op, threshold, errParse := ParseConditionJSON(tc.conditionJSON)
			if tc.wantErr {
				if errParse == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if errParse != nil {
				t.Fatalf("unexpected error: %v", errParse)
			}
			if op != tc.wantOp {
				t.Errorf("operator = %q, want %q", op, tc.wantOp)
			}
			if threshold != tc.wantThreshold {
				t.Errorf("threshold = %v, want %v", threshold, tc.wantThreshold)
			}
		})
	}
}

func TestParseThresholdConditionJSON(t *testing.T) {
	tests := []struct {
		name          string
		conditionJSON string
		want          ThresholdCondition
		wantErr       bool
	}{
		{
			name:          "extended timing and recovery policy",
			conditionJSON: `{"operator":">=","value":80,"for_seconds":30,"recovery_value":70,"cooldown_seconds":60,"repeat_seconds":300,"no_data_seconds":120}`,
			want: ThresholdCondition{
				Operator:        ">=",
				Value:           80,
				ForSeconds:      30,
				CooldownSeconds: 60,
				RepeatSeconds:   300,
				NoDataSeconds:   120,
			},
		},
		{
			name:          "negative duration rejected",
			conditionJSON: `{"operator":">=","value":80,"for_seconds":-1}`,
			wantErr:       true,
		},
		{
			name:          "unknown operator rejected",
			conditionJSON: `{"operator":"!=","value":80}`,
			wantErr:       true,
		},
		{
			name:          "high threshold recovery must be lower",
			conditionJSON: `{"operator":">=","value":80,"recovery_value":80}`,
			wantErr:       true,
		},
		{
			name:          "low threshold recovery must be higher",
			conditionJSON: `{"operator":"<=","value":20,"recovery_value":10}`,
			wantErr:       true,
		},
		{
			name:          "equality threshold rejects recovery value",
			conditionJSON: `{"operator":"==","value":20,"recovery_value":10}`,
			wantErr:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, errParse := ParseThresholdConditionJSON(tc.conditionJSON)
			if tc.wantErr {
				if errParse == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if errParse != nil {
				t.Fatalf("unexpected error: %v", errParse)
			}
			if got.Operator != tc.want.Operator || got.Value != tc.want.Value || got.ForSeconds != tc.want.ForSeconds ||
				got.CooldownSeconds != tc.want.CooldownSeconds || got.RepeatSeconds != tc.want.RepeatSeconds ||
				got.NoDataSeconds != tc.want.NoDataSeconds {
				t.Fatalf("condition = %+v, want %+v", got, tc.want)
			}
			if got.RecoveryValue == nil || *got.RecoveryValue != 70 {
				t.Fatalf("recovery value = %v, want 70", got.RecoveryValue)
			}
		})
	}
}
