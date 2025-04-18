package util

import "testing"

func TestStrictAtoi(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		hasError bool
	}{
		{input: "123", expected: 123, hasError: false},
		{input: "-123", expected: -123, hasError: false},
		{input: "0", expected: 0, hasError: false},
		{input: "+123", expected: 0, hasError: true},
		{input: "+0", expected: 0, hasError: true},
		{input: "-0", expected: 0, hasError: true},
		{input: "001", expected: 0, hasError: true},
		{input: "+001", expected: 0, hasError: true},
		{input: "-001", expected: 0, hasError: true},
		{input: "abc", expected: 0, hasError: true},
		{input: "", expected: 0, hasError: true},
	}

	for _, test := range tests {
		result, err := StrictAtoi(test.input)
		if (err != nil) != test.hasError {
			t.Errorf("StrictAtoi(%q) error = %v, wantErr %v", test.input, err, test.hasError)
		}
		if result != test.expected {
			t.Errorf("StrictAtoi(%q) = %v, want %v", test.input, result, test.expected)
		}
	}
}
