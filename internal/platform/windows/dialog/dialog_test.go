package dialog

import "testing"

func TestCompose(t *testing.T) {
	tests := []struct {
		heading string
		body    string
		want    string
	}{
		{"", "body", "body"},
		{"heading", "", "heading"},
		{"heading", "body", "heading\r\n\r\nbody"},
	}
	for _, tt := range tests {
		if got := compose(tt.heading, tt.body); got != tt.want {
			t.Fatalf("compose(%q, %q) = %q, want %q", tt.heading, tt.body, got, tt.want)
		}
	}
}
