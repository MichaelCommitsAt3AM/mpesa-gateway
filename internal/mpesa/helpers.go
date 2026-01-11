package mpesa

// Item represents a key-value pair from M-Pesa callback metadata
type Item struct {
	Name  string      `json:"Name"`
	Value interface{} `json:"Value"`
}

// ParseMpesaMetadata converts M-Pesa's metadata array to a clean map
// Input example: [{"Name": "Amount", "Value": 100}, {"Name": "MpesaReceiptNumber", "Value": "ABC123"}]
// Output: {"Amount": 100, "MpesaReceiptNumber": "ABC123"}
func ParseMpesaMetadata(items []Item) map[string]interface{} {
	result := make(map[string]interface{}, len(items))
	for _, item := range items {
		if item.Name != "" {
			result[item.Name] = item.Value
		}
	}
	return result
}
