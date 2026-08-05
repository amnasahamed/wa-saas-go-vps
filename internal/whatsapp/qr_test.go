package whatsapp

import (
	"strings"
	"testing"
)

func TestRenderQRDataURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, got string)
	}{
		{
			name:    "empty input returns empty string, no error",
			input:   "",
			wantErr: false,
			check: func(t *testing.T, got string) {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
			},
		},
		{
			name:    "valid QR string returns a data URL",
			input:   "1@abc123,def456,ghi789,0",
			wantErr: false,
			check: func(t *testing.T, got string) {
				if !strings.HasPrefix(got, "data:image/png;base64,") {
					t.Errorf("missing data URL prefix: %q", got)
				}
				if len(got) < 200 {
					t.Errorf("PNG payload looks too small: %d chars", len(got))
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderQRDataURL(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			tc.check(t, got)
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"919876543210", "919876543210", false},
		{"+91 987 654 3210", "919876543210", false},
		{"9876543210", "919876543210", false}, // 10-digit + India default
		{"12345678901234", "", true},         // 14 digits => LID pattern
		{"", "", true},
		{"foo@bar", "", true},                // 6 digits, no default match
	}

	for _, tc := range tests {
		got, err := NormalizePhone(tc.in, "91")
		if (err != nil) != tc.wantErr {
			t.Errorf("NormalizePhone(%q) err = %v, wantErr = %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLast10Digits(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"919876543210", "9876543210"},
		{"12345", "12345"},
		{"abcdefg1234567890", "1234567890"},
	}
	for _, tc := range tests {
		if got := Last10Digits(tc.in); got != tc.want {
			t.Errorf("Last10Digits(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}