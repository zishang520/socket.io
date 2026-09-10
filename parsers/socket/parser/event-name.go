package parser

import "encoding/json"

// EventName converts a validated event name to its dispatch key.
// Numeric names use JavaScript's decimal/exponent thresholds, including -0 as "0".
func EventName(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		if v == 0 {
			return "0"
		}
	case int, int32, int64:
	default:
		return ""
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
