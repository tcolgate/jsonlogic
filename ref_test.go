package jsonlogic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDottedRef(t *testing.T) {
	tests := []struct {
		name   string
		data   any
		ref    any
		expect any
	}{
		{
			name:   "nil",
			data:   nil,
			ref:    "blah",
			expect: nil,
		},
		{
			name:   "non-map",
			data:   2.0,
			ref:    "blah",
			expect: nil,
		},
		{
			name:   "bad-key",
			data:   []any{2.0},
			ref:    false,
			expect: nil,
		},
		{
			name:   "single-found",
			data:   map[string]any{"one": 2.0},
			ref:    "one",
			expect: 2.0,
		},
		{
			name:   "single-failed",
			data:   map[string]any{"one": 2.0},
			ref:    "two",
			expect: nil,
		},
		{
			name:   "deep-found",
			data:   map[string]any{"one": map[string]any{"two": 2.0}},
			ref:    "one.two",
			expect: 2.0,
		},
		{
			name:   "deep-miss",
			data:   map[string]any{"one": map[string]any{"two": 2.0}},
			ref:    "one.three",
			expect: nil,
		},
		{
			name:   "deep-non-trivial",
			data:   map[string]any{"one": map[string]any{"two": []any{"hello", 2.0}}},
			ref:    "one.two",
			expect: []any{"hello", 2.0},
		},
		{
			name:   "deep-non-trivial",
			data:   map[string]any{"one": map[string]any{"two": []any{"hello", 2.0}}},
			ref:    "one.two.0",
			expect: "hello",
		},
		{
			name:   "deep-array",
			data:   map[string]any{"one": map[string]any{"two": []any{"hello", 2.0}}},
			ref:    "one.two.3",
			expect: nil,
		},
		{
			name:   "deep-non-trivial",
			data:   map[string]any{"one": map[string]any{"two": []any{[]any{"hello", 2.0}}}},
			ref:    "one.two.0.1",
			expect: 2.0,
		},
		{
			name:   "array-float",
			data:   []any{"hello", 2.0},
			ref:    1.0,
			expect: 2.0,
		},
		{
			name:   "array-string",
			data:   []any{"hello", 2.0},
			ref:    "1",
			expect: 2.0,
		},
		{
			name:   "array-non-int",
			data:   []any{"hello", 2.0},
			ref:    1.5,
			expect: nil,
		},
	}

	for _, st := range tests {
		t.Run(st.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				v := DottedRef(st.data, st.ref)
				assert.Equal(t, st.expect, v)
			})
		})
	}
}
