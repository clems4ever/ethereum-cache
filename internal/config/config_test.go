package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		hasError bool
	}{
		{"100", 100, false},
		{"100k", 100 * 1024, false},
		{"100K", 100 * 1024, false},
		{"100kb", 100 * 1024, false},
		{"100KB", 100 * 1024, false},
		{"100m", 100 * 1024 * 1024, false},
		{"100M", 100 * 1024 * 1024, false},
		{"100mb", 100 * 1024 * 1024, false},
		{"100MB", 100 * 1024 * 1024, false},
		{"100g", 100 * 1024 * 1024 * 1024, false},
		{"100G", 100 * 1024 * 1024 * 1024, false},
		{"100gb", 100 * 1024 * 1024 * 1024, false},
		{"100GB", 100 * 1024 * 1024 * 1024, false},
		{"", 0, false},
		{"invalid", 0, true},
		{"100x", 0, true},
	}

	for _, test := range tests {
		val, err := ParseBytes(test.input)
		if test.hasError {
			assert.Error(t, err, "input: %s", test.input)
		} else {
			assert.NoError(t, err, "input: %s", test.input)
			assert.Equal(t, test.expected, val, "input: %s", test.input)
		}
	}
}
