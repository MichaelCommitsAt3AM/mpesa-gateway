package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidTransition(t *testing.T) {
	allStates := []TransactionStatus{StatusPending, StatusCompleted, StatusFailed}

	allowed := map[TransactionStatus]map[TransactionStatus]bool{
		StatusPending: {StatusCompleted: true, StatusFailed: true},
	}

	for _, from := range allStates {
		for _, to := range allStates {
			want := allowed[from][to]
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				assert.Equal(t, want, IsValidTransition(from, to))
			})
		}
	}
}

func TestIsValidTransition_UnknownFromState(t *testing.T) {
	assert.False(t, IsValidTransition(TransactionStatus("BOGUS"), StatusCompleted))
}
