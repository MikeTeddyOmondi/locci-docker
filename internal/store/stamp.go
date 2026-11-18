package store

import (
	"fmt"
	"time"
)

// stamp scans a timestamp column into a string in RFC 3339 (UTC), whatever
// form the driver hands over: SQLite's "2006-01-02 15:04:05" text from
// CURRENT_TIMESTAMP, RFC 3339 text, or a time.Time. NULL becomes "".
type stamp struct{ dst *string }

func (s stamp) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		*s.dst = ""
	case time.Time:
		*s.dst = x.UTC().Format(time.RFC3339)
	case string:
		*s.dst = normStamp(x)
	case []byte:
		*s.dst = normStamp(string(x))
	default:
		return fmt.Errorf("timestamp column: unexpected %T", v)
	}
	return nil
}

func normStamp(v string) string {
	if t, ok := parseTime(v); ok {
		return t.UTC().Format(time.RFC3339)
	}
	return v
}

// parseTime reads a timestamp as SQLite stores it or in RFC 3339.
func parseTime(v string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
