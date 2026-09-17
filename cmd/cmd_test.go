package cmd

import "testing"

func TestEnvPort(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want int
	}{
		{"unset", "", 8080},
		{"not a number", "eighty", 8080},
		{"zero", "0", 8080},
		{"above the port range", "65536", 8080},
		{"inside the range", "9090", 9090},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("INOICHI_PORT", tt.env)
			if got := envPort(); got != tt.want {
				t.Errorf("envPort() = %d, want %d", got, tt.want)
			}
		})
	}
}
