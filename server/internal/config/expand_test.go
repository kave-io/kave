package config

import "testing"

func TestExpand(t *testing.T) {
	env := map[string]string{
		"HOST": "127.0.0.1",
		"PORT": "7777",
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "plain",
			input: "daemon: ${HOST}:${PORT}",
			want:  "daemon: 127.0.0.1:7777",
		},
		{
			name:  "default",
			input: "daemon: ${MISSING:-127.0.0.1:8080}",
			want:  "daemon: 127.0.0.1:8080",
		},
		{
			name:    "required",
			input:   "daemon: ${MISSING:?set MISSING}",
			wantErr: true,
		},
		{
			name:  "escape",
			input: "literal: $$HOST",
			want:  "literal: $HOST",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Expand(tc.input, "config.yaml", env)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Expand() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("Expand() = %q, want %q", got, tc.want)
			}
		})
	}
}
