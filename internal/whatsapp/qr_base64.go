package whatsapp

import "encoding/base64"

// base64Encode wraps stdlib base64 to keep the qr.go imports focused on
// the go-qrcode dependency (avoids leaking the encoding/base64 import
// across the rest of the package).
func base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
