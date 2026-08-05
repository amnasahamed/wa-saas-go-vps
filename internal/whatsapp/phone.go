// Package whatsapp wraps the whatsmeow client and provides multi-tenant
// session management, QR login flows, media download/upload to S3, and the
// phone normalization logic ported from the legacy whatsapp-bridge Node app.
package whatsapp

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidPhone is returned when a phone cannot be normalized.
var ErrInvalidPhone = errors.New("invalid phone number")

// nonDigits matches anything that isn't a digit.
var nonDigits = regexp.MustCompile(`\D`)

// lidPattern matches WhatsApp LIDs (long identifiers that aren't phone numbers).
// LIDs are >13 digits and don't correspond to a real phone.
var lidPattern = regexp.MustCompile(`^[0-9]{14,}$`)

// NormalizePhone strips the WhatsApp suffix (e.g. "@c.us", "@g.us", "@lid"),
// removes non-digits, and applies the country-code default for short numbers.
//
// Ported from whatsapp-bridge/index.js lines 62-74 with the India 91 prefix
// default. The country prefix is now per-tenant (configurable), so we accept
// it as a parameter rather than hardcoding "91".
//
// Returns ErrInvalidPhone if the result is empty or exceeds 15 digits (E.164 max).
func NormalizePhone(raw, defaultCountryCode string) (string, error) {
	stripped := stripWhatsAppSuffix(raw)
	digits := nonDigits.ReplaceAllString(stripped, "")

	if digits == "" {
		return "", ErrInvalidPhone
	}

	// Reject LIDs (long identifiers that aren't real phone numbers)
	if lidPattern.MatchString(digits) {
		return "", ErrInvalidPhone
	}

	// If already has country code (12-15 digits), use as-is
	if len(digits) >= 12 && len(digits) <= 15 {
		return digits, nil
	}

	// If 10-digit, prefix with default country code (India use case)
	if len(digits) == 10 {
		if defaultCountryCode == "" {
			defaultCountryCode = "91"
		}
		return defaultCountryCode + digits, nil
	}

	// 11-13 digits: assume missing leading digit; prefix country code
	if len(digits) >= 11 && len(digits) <= 13 {
		if defaultCountryCode == "" {
			defaultCountryCode = "91"
		}
		return defaultCountryCode + digits, nil
	}

	return "", ErrInvalidPhone
}

// stripWhatsAppSuffix removes "@c.us", "@g.us", "@lid" etc.
func stripWhatsAppSuffix(s string) string {
	if idx := strings.Index(s, "@"); idx >= 0 {
		return s[:idx]
	}
	return s
}

// Last10Digits returns the last 10 digits of a phone number, used for
// match-by-phone logic in destination upserts (Sheets/Airtable/Notion rows
// can have inconsistent country prefixes).
func Last10Digits(phone string) string {
	digits := nonDigits.ReplaceAllString(phone, "")
	if len(digits) < 10 {
		return digits
	}
	return digits[len(digits)-10:]
}

// IsVoiceNoteType returns true for whatsmeow message types that represent
// audio/voice notes. Used by the nurture layer to suppress auto-reply and
// mark the lead for manual follow-up (current behavior, since Whisper is
// out of MVP).
func IsVoiceNoteType(messageType string) bool {
	switch strings.ToLower(messageType) {
	case "audio", "ptt", "voice", "audiomessage":
		return true
	}
	return false
}

// IsVoiceMIME returns true for MIME types that represent voice notes.
func IsVoiceMIME(mimetype string) bool {
	switch strings.ToLower(mimetype) {
	case "audio/ogg", "audio/opus", "audio/mpeg", "audio/mp4", "audio/aac":
		return true
	}
	return false
}
