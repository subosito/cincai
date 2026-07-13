package fields

import "strings"

func String(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}