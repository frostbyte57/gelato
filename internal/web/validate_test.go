package web

import "testing"

func TestValidateHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{name: "ipv4", host: "127.0.0.1"},
		{name: "hostname", host: "localhost"},
		{name: "ipv6", host: "::1"},
		{name: "empty", host: "", wantErr: true},
		{name: "whitespace", host: "bad host", wantErr: true},
		{name: "hostport", host: "127.0.0.1:9000", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHost(tt.host)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateHost(%q) error=%v wantErr=%t", tt.host, err, tt.wantErr)
			}
		})
	}
}
