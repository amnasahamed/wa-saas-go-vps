package whatsapp

import (
	"testing"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

func TestClassifyInbound(t *testing.T) {
	cases := []struct {
		name     string
		in       *waE2E.Message
		wantType string
		wantBody string
	}{
		{"nil", nil, "unknown", ""},
		{"text", &waE2E.Message{Conversation: ptr("hello")}, "text", "hello"},
		{"extended text", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: ptr("hi")}}, "text", "hi"},
		{"image with caption", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: ptr("see this"), Mimetype: ptr("image/jpeg")}}, "image", "see this"},
		{"audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{Mimetype: ptr("audio/ogg")}}, "audio", ""},
		{"unknown", &waE2E.Message{}, "unknown", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotBody, _, _ := classifyInbound(tc.in)
			if gotType != tc.wantType || gotBody != tc.wantBody {
				t.Errorf("classifyInbound(%s) = (%q, %q), want (%q, %q)",
					tc.name, gotType, gotBody, tc.wantType, tc.wantBody)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
