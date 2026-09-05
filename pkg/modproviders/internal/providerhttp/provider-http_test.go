package providerhttp

import (
	"slices"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
)

func TestStringParam(t *testing.T) {
	tests := []struct {
		name   string
		params modproviders.SearchParams
		want   string
	}{
		{name: "nil params"},
		{name: "missing key", params: modproviders.SearchParams{"other": "value"}},
		{name: "nil value", params: modproviders.SearchParams{"key": nil}},
		{name: "wrong type", params: modproviders.SearchParams{"key": 42}},
		{name: "empty string", params: modproviders.SearchParams{"key": ""}},
		{name: "string unchanged", params: modproviders.SearchParams{"key": " value "}, want: " value "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := StringParam(test.params, "key")
			if got != test.want {
				t.Errorf("StringParam() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestIntParam(t *testing.T) {
	tests := []struct {
		name   string
		params modproviders.SearchParams
		want   int
	}{
		{name: "nil params", want: 20},
		{name: "missing key", params: modproviders.SearchParams{"other": 42}, want: 20},
		{name: "nil value", params: modproviders.SearchParams{"key": nil}, want: 20},
		{name: "wrong type", params: modproviders.SearchParams{"key": int64(42)}, want: 20},
		{name: "zero", params: modproviders.SearchParams{"key": 0}},
		{name: "positive", params: modproviders.SearchParams{"key": 42}, want: 42},
		{name: "negative", params: modproviders.SearchParams{"key": -1}, want: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := IntParam(test.params, "key", 20)
			if got != test.want {
				t.Errorf("IntParam() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestStringSliceParam(t *testing.T) {
	tests := []struct {
		name   string
		params modproviders.SearchParams
		want   []string
	}{
		{name: "nil params"},
		{name: "missing key", params: modproviders.SearchParams{"other": []string{"value"}}},
		{name: "nil value", params: modproviders.SearchParams{"key": nil}},
		{name: "wrong type", params: modproviders.SearchParams{"key": []any{"value"}}},
		{name: "typed nil", params: modproviders.SearchParams{"key": []string(nil)}},
		{name: "empty slice", params: modproviders.SearchParams{"key": []string{}}, want: []string{}},
		{name: "nonempty slice", params: modproviders.SearchParams{"key": []string{"one", "two"}}, want: []string{"one", "two"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := StringSliceParam(test.params, "key")
			if (got == nil) != (test.want == nil) || !slices.Equal(got, test.want) {
				t.Errorf("StringSliceParam() = %#v, want %#v", got, test.want)
			}
		})
	}
}
