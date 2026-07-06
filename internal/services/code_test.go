package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	svc := NewCodeService()

	t.Run("generates 6-character code", func(t *testing.T) {
		code := svc.Generate()
		assert.Len(t, code, 6)
	})

	t.Run("generates valid code", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			code := svc.Generate()
			assert.True(t, svc.IsValid(code))
		}
	})

	t.Run("generates different codes", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 50; i++ {
			code := svc.Generate()
			assert.False(t, seen[code], "duplicate code: %s", code)
			seen[code] = true
		}
	})
}

func TestIsValid(t *testing.T) {
	t.Parallel()

	svc := NewCodeService()

	tests := []struct {
		name  string
		code  string
		valid bool
	}{
		{"valid code", "ABCDEF", true},
		{"valid alphanumeric without ambiguous chars", "KHYM3A", true},
		{"upper/lower mix", "AbCdEf", true},
		{"lowercase", "abcdef", true},
		{"too short", "ABCDE", false},
		{"too long", "ABCDEFG", false},
		{"contains digit 0", "AB0DEF", false},
		{"contains digit 1", "A1CDEF", false},
		{"contains letter O", "ABCDEO", false},
		{"contains letter I", "IBCDEF", false},
		{"contains special char", "ABC@EF", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.IsValid(tt.code)
			assert.Equal(t, tt.valid, got)
		})
	}
}
