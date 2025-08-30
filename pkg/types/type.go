package types

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type (
	Void = struct{}

	Callable = func()

	HttpCompression struct {
		Threshold int `json:"threshold,omitempty" msgpack:"threshold,omitempty"`
	}

	PerMessageDeflate struct {
		Threshold int `json:"threshold,omitempty" msgpack:"threshold,omitempty"`
	}
)

var (
	NULL Void
)

type IncomingHttpHeaders map[string]any // HTTP headers where any represents either string or []string

func (h IncomingHttpHeaders) Header() http.Header {
	if h == nil {
		return nil
	}
	header := make(http.Header, len(h))
	for key, value := range h {
		canonicalKey := http.CanonicalHeaderKey(key)
		if values := convertToStringSlice(value); len(values) > 0 {
			header[canonicalKey] = values
		}
	}

	return header
}

type ParsedUrlQuery map[string]any // Query parameters where any represents either string or []string

func (q ParsedUrlQuery) Query() url.Values {
	if q == nil {
		return nil
	}
	values := make(url.Values, len(q))
	for key, value := range q {
		if vals := convertToStringSlice(value); len(vals) > 0 {
			values[key] = vals
		}
	}

	return values
}

func convertToStringSlice(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str := convertAnyToString(item); str != "" {
				result = append(result, str)
			}
		}
		return result
	default:
		if str := convertAnyToString(v); str != "" {
			return []string{str}
		}
		return nil
	}
}

func convertAnyToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%g", v)
	case bool:
		return strconv.FormatBool(v)
	case fmt.Stringer:
		return v.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}
}

// noCopy may be added to structs which must not be copied
// after the first use.
//
// See https://golang.org/issues/8005#issuecomment-190753527
// for details.
//
// Note that it must not be embedded, due to the Lock and Unlock methods.
type noCopy struct{}

// Lock is a no-op used by -copylocks checker from `go vet`.
func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}
