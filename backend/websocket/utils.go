package websocket

// getString safely extracts a string value from a map[string]interface{}.
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// getFloat64 safely extracts a float64 value from a map[string]interface{}.
func getFloat64(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return 0
}

// getInt64 safely extracts an int64 value from a map[string]interface{}.
// Finnhub often sends numeric values as float64, so we handle that case.
func getInt64(m map[string]interface{}, key string) int64 {
	if val, ok := m[key].(float64); ok {
		return int64(val)
	}
	return 0
}