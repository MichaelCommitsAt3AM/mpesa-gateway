package payment

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// TestFormatAmountForSafaricom locks down the rounding behavior for the
// value actually sent to Safaricom. This is money: a silent change in
// rounding here (e.g. from a decimal library upgrade) would mean charging
// customers a different amount than what's stored in our own database.
func TestFormatAmountForSafaricom(t *testing.T) {
	tests := []struct {
		name   string
		amount string
		want   string
	}{
		{"whole number", "100", "100"},
		{"rounds up at .50", "100.50", "101"},
		{"rounds down below .50", "100.49", "100"},
		{"rounds up above .50", "100.51", "101"},
		{"small amount rounds up", "0.50", "1"},
		{"small amount rounds down", "0.49", "0"},
		{"large amount", "150000.99", "150001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount, err := decimal.NewFromString(tt.amount)
			if err != nil {
				t.Fatalf("failed to parse test amount %q: %v", tt.amount, err)
			}
			assert.Equal(t, tt.want, formatAmountForSafaricom(amount))
		})
	}
}
