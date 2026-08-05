package whatsapp

import (
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

// buildTextMessage constructs a minimal text message envelope for
// whatsmeow.SendMessage. whatsmeow auto-attaches the device's identity
// on the wire; we just provide the conversation payload.
//
// Used by Manager.SendMessage for outbound text-only flows. Voice and
// document flows land in a follow-up.
func buildTextMessage(body string) *waE2E.Message {
	return &waE2E.Message{
		Conversation: &body,
	}
}
