package request

import (
	"net/http"
	"strconv"
	"testing"

	"resty.dev/v3"
)

func TestResponseOk(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		{0, false},
		{200, true},
		{201, true},
		{202, true},
		{204, true},
		{299, true},
		{199, false},
		{300, false},
		{301, false},
		{302, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{500, false},
		{501, false},
		{502, false},
		{503, false},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.statusCode), func(t *testing.T) {
			response := &Response{&resty.Response{RawResponse: &http.Response{StatusCode: tt.statusCode}}}
			ok := response.Ok()
			if ok != tt.expected {
				t.Errorf("StatusCode %d: expected %v, got %v", tt.statusCode, tt.expected, ok)
			}
		})
	}
}
