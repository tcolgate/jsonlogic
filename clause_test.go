package jsonlogic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClauseMarshal(t *testing.T) {
	type test struct {
		name      string
		rule      string
		marshalTo string
		parseErr  string
	}

	tests := []test{
		{
			name:     "invalid-json",
			rule:     `{`,
			parseErr: "unexpected end of JSON input",
		},
		{ // invalid clause, but valid JSON, should return the thing passed in.
			name:      "invalid-clause",
			rule:      `"hello"`,
			marshalTo: `"hello"`,
		},
		{
			name:      "invalid-clause2",
			rule:      `{"var":["a"],"hola":"thing"}`,
			marshalTo: `{"hola":"thing","var":["a"]}`,
		},
		{
			name:      "always",
			rule:      `true`,
			marshalTo: `true`,
		},
		{
			name:      "never",
			rule:      `false`,
			marshalTo: `false`,
		},
		{
			name:      "simple",
			rule:      `{ "==" : [1, 1] }`,
			marshalTo: `{"==":[1,1]}`,
		},
		{
			name: "compound",
			rule: `{"and" : [
								 { ">" : [3,1] },
								 { "<" : [1,3] }
							 ] }`,
			marshalTo: `{"and":[{"\u003e":[3,1]},{"\u003c":[1,3]}]}`,
		},
		{
			name:      "data-driven",
			rule:      `{ "var" : ["a"] }`,
			marshalTo: `{"var":["a"]}`,
		},
		{
			name:      "data-driven-sugar",
			rule:      `{ "var" : "a" }`,
			marshalTo: `{"var":["a"]}`,
		},
	}

	for _, st := range tests {
		t.Run(st.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				var c Clause
				err := json.Unmarshal([]byte(st.rule), &c)
				if st.parseErr != "" {
					assert.EqualErrorf(t, err, st.parseErr, "unmarshal error")
					return
				}
				assert.NoErrorf(t, err, "unmarshal error")

				marshalTo, err := json.Marshal(c)
				assert.NoErrorf(t, err, "marshal error")
				assert.Equalf(t, st.marshalTo, string(marshalTo), "re-marshaled clause")
			})
		})
	}
}

func TestClauseEval(t *testing.T) {
	type test struct {
		name       string
		rule       string
		data       interface{}
		expect     interface{}
		compileErr string
	}

	tests := []test{
		{ // invalid clause, but valid JSON, should return the thing passed in.
			name:   "invalid-clause",
			rule:   `"hello"`,
			expect: "hello",
		},
		{
			name: "invalid-clause2",
			rule: `{"var":["a"],"hola":"thing"}`,
			expect: map[string]interface{}{
				"hola": "thing",
				"var":  []interface{}{"a"},
			},
		},
		{
			name:   "always",
			rule:   `true`,
			expect: true,
		},
		{
			name:   "never",
			rule:   `false`,
			expect: false,
		},
		{
			name:       "unknown-op",
			rule:       `{ "XXX" : [1, 1] }`,
			compileErr: "unrecognized operation XXX",
		},
		{
			name:   "simple",
			rule:   `{ "==" : [1, 1] }`,
			expect: true,
		},
		{
			name:   "simple-uncoerced",
			rule:   `{ "===" : [1, 1] }`,
			expect: true,
		},
		{
			name: "compound",
			rule: `{"and" : [
								 { ">" : [3,1] },
								 { "<" : [1,3] }
							 ] }`,
			expect: true,
		},
		{
			name:   "data-driven",
			rule:   `{ "var" : ["a"] }`,
			data:   map[string]interface{}{"a": float64(1)},
			expect: float64(1),
		},
		{
			name:   "data-driven-default",
			rule:   `{ "var" : ["b",26] }`,
			data:   map[string]interface{}{"a": float64(1)},
			expect: float64(26),
		},
		{
			name:   "data-driven-sugar",
			rule:   `{ "var" : "a" }`,
			data:   map[string]interface{}{"a": float64(1)},
			expect: float64(1),
		},
		{
			name:   "data-driven-empty-key",
			rule:   `{ "var" : "" }`,
			data:   map[string]interface{}{"a": float64(1)},
			expect: map[string]interface{}{"a": float64(1)},
		},
		{
			name:   "data-driven-array",
			rule:   `{ "var" : 1 }`,
			data:   []interface{}{"apple", "banana", "carrot"},
			expect: "banana",
		},
		{
			name: "data-driven-mixed",
			rule: `{ "and" : [
									{"<" : [ { "var" : "temp" }, 110 ]},
									{"==" : [ { "var" : "pie.filling" }, "apple" ] }
								] }`,
			data: map[string]interface{}{
				"temp": 100.0,
				"pie":  map[string]interface{}{"filling": "apple"},
			},
			expect: true,
		},
		{
			name: "fizzbuzz",
			rule: `{ "if": [
								{"==": [ { "%": [ { "var": "i" }, 15 ] }, 0]},
								"fizzbuzz",

								{"==": [ { "%": [ { "var": "i" }, 3 ] }, 0]},
								"fizz",

								{"==": [ { "%": [ { "var": "i" }, 5 ] }, 0]},
								"buzz",

								{ "var": "i" }
							 ]}`,
			expect: "buzz",
			data:   map[string]interface{}{"i": float64(20)},
		},
		{
			name:   "missing",
			rule:   `{"missing":["a", "b"]}`,
			data:   map[string]interface{}{"a": "apple", "c": "carrot"},
			expect: []interface{}{"b"},
		},
	}

	for _, st := range tests {
		t.Run(st.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				var c Clause
				err := json.Unmarshal([]byte(st.rule), &c)
				assert.NoErrorf(t, err, "unmarshal error")

				cf, err := Compile(&c)
				if st.compileErr != "" {
					assert.EqualErrorf(t, err, st.compileErr, "compile error")
					if err != nil {
						return
					}
				} else {
					assert.NoErrorf(t, err, "compile error")
				}

				v := cf(st.data)
				assert.Equalf(t, st.expect, v, "response data")
			})
		})
	}
}

func BenchmarkFizzBuzz(b *testing.B) {
	b.ReportAllocs()
	fizzbuzz := `{ "if": [
								{"==": [ { "%": [ { "var": "i" }, 15 ] }, 0]},
								"fizzbuzz",

								{"==": [ { "%": [ { "var": "i" }, 3 ] }, 0]},
								"fizz",

								{"==": [ { "%": [ { "var": "i" }, 5 ] }, 0]},
								"buzz",

								{ "var": "i" }
							 ]}`
	data := map[string]interface{}{"i": float64(20)}

	var c Clause
	err := json.Unmarshal([]byte(fizzbuzz), &c)
	if err != nil {
		b.Fatalf("unmarshal failed, %v", err)
	}

	cf, err := Compile(&c)
	if err != nil {
		b.Fatalf("compile failed, %v", err)
	}
	b.ResetTimer()
	for i := b.N; i >= 0; i-- {
		cf(data)
	}
}
