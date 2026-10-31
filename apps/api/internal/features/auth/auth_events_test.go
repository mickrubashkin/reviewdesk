package auth

import (
	"net/http/httptest"
	"testing"
)

func TestRequestIPAddress(t *testing.T) {
	for _, test := range []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"behind cloudflare", map[string]string{"CF-Connecting-IP": "203.0.113.7", "X-Forwarded-For": "162.158.1.1"}, "203.0.113.7"},
		{"forwarded", map[string]string{"X-Forwarded-For": "203.0.113.7, 10.0.0.1"}, "203.0.113.7"},
		{"real ip", map[string]string{"X-Real-IP": "203.0.113.7"}, "203.0.113.7"},
		{"direct", nil, "192.0.2.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			for name, value := range test.headers {
				r.Header.Set(name, value)
			}
			if got := RequestIPAddress(r); got != test.want {
				t.Errorf("RequestIPAddress = %q, want %q", got, test.want)
			}
		})
	}
}
