package roon

import (
	"fmt"
	"strconv"
)

func parseSubscriptionKey(v any) string {
	switch x := v.(type) {
	case nil:
		return "0"
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case string:
		if x == "" {
			return "0"
		}
		return x
	default:
		return fmt.Sprint(x)
	}
}
