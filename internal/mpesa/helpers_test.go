package mpesa

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMpesaMetadata(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		result := ParseMpesaMetadata(nil)
		assert.Empty(t, result)
	})

	t.Run("typical callback items", func(t *testing.T) {
		items := []Item{
			{Name: "Amount", Value: float64(100)},
			{Name: "MpesaReceiptNumber", Value: "OEI2AK3ZQO"},
			{Name: "TransactionDate", Value: float64(20240111135500)},
			{Name: "PhoneNumber", Value: float64(254712345678)},
		}
		result := ParseMpesaMetadata(items)
		assert.Equal(t, float64(100), result["Amount"])
		assert.Equal(t, "OEI2AK3ZQO", result["MpesaReceiptNumber"])
		assert.Len(t, result, 4)
	})

	t.Run("item with empty name is skipped", func(t *testing.T) {
		items := []Item{
			{Name: "", Value: "should not appear"},
			{Name: "Amount", Value: float64(50)},
		}
		result := ParseMpesaMetadata(items)
		assert.Len(t, result, 1)
		assert.Equal(t, float64(50), result["Amount"])
	})

	t.Run("duplicate names: last one wins", func(t *testing.T) {
		items := []Item{
			{Name: "Amount", Value: float64(1)},
			{Name: "Amount", Value: float64(2)},
		}
		result := ParseMpesaMetadata(items)
		assert.Equal(t, float64(2), result["Amount"])
	})

	t.Run("nil value is preserved, not dropped", func(t *testing.T) {
		items := []Item{
			{Name: "MpesaReceiptNumber", Value: nil},
		}
		result := ParseMpesaMetadata(items)
		_, exists := result["MpesaReceiptNumber"]
		assert.True(t, exists, "key should exist even with a nil value")
	})
}
