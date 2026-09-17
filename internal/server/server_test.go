package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{"no origin header", "", true},
		{"same host", "http://example.com", true},
		{"same host on another scheme", "https://example.com", true},
		{"another host", "http://evil.test", false},
		{"same host with a port", "http://example.com:8080", false},
		{"unparsable origin", "://", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/maps", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if got := sameOrigin(req); got != tt.want {
				t.Errorf("sameOrigin(Origin=%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}
