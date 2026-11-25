package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvaluationAnswerRequiresExactJSON(t *testing.T) {
	for _, test := range []struct {
		name, answer string
		passed       bool
	}{
		{"correct", `{"ids":["a","c","b"],"total":41}`, true},
		{"different key order", `{"total":41,"ids":["a","c","b"]}`, true},
		{"wrong array order", `{"ids":["c","a","b"],"total":41}`, false},
		{"extra fields", `{"ids":["a","c","b"],"total":41,"extra":1}`, false},
		{"answer in prose", `The answer is {"ids":["a","c","b"],"total":41}`, false},
		{"string instead of number", `{"ids":["a","c","b"],"total":"41"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.passed, GradeEvaluationAnswer(test.answer, `{"ids":["a","c","b"],"total":41}`))
		})
	}
}
