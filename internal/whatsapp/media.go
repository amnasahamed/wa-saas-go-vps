package whatsapp

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

// Allowed asset extensions (port from whatsapp-bridge/index.js:452-469).
// Anything outside this set is rejected at the API boundary.
var allowedAssetExtensions = map[string]struct{}{
	".pdf": {}, ".png": {}, ".jpg": {}, ".jpeg": {},
	".gif": {}, ".webp": {}, ".mp3": {}, ".ogg": {},
	".opus": {}, ".m4a": {}, ".mp4": {}, ".mov": {},
	".webm": {}, ".doc": {}, ".docx": {}, ".xls": {},
	".xlsx": {}, ".txt": {},
}

// mimeFromExtension maps file extensions to MIME types.
// Hand-rolled for the asset allowlist; outbound sends accept arbitrary
// MIME in MediaS3Key metadata.
var mimeFromExtension = map[string]string{
	".pdf":  "application/pdf",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".mp3":  "audio/mpeg",
	".ogg":  "audio/ogg",
	".opus": "audio/ogg",
	".m4a":  "audio/mp4",
	".mp4":  "video/mp4",
	".mov":  "video/quicktime",
	".webm": "video/webm",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".txt":  "text/plain",
}

// extensionFromMIME maps MIME to a default extension when uploading media
// without an explicit filename.
var extensionFromMIME = map[string]string{
	"application/pdf": "pdf",
	"image/png":       "png",
	"image/jpeg":      "jpg",
	"image/gif":       "gif",
	"image/webp":      "webp",
	"audio/mpeg":      "mp3",
	"audio/ogg":       "ogg",
	"audio/opus":      "ogg",
	"audio/mp4":       "m4a",
	"video/mp4":       "mp4",
	"video/quicktime": "mov",
	"video/webm":      "webm",
}

// SanitizeAssetFilename validates and cleans an uploaded asset filename.
// Returns the cleaned filename or an error if it's outside the allowlist.
//
// Ported from whatsapp-bridge/index.js:471-480.
var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// ErrInvalidFilename is returned for empty, dotfile, or disallowed filenames.
var ErrInvalidFilename = errors.New("invalid or disallowed filename")

func SanitizeAssetFilename(name string) (string, error) {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "" || strings.HasPrefix(base, ".") {
		return "", ErrInvalidFilename
	}
	safe := unsafeChars.ReplaceAllString(base, "_")
	ext := strings.ToLower(filepath.Ext(safe))
	if _, ok := allowedAssetExtensions[ext]; !ok {
		return "", ErrInvalidFilename
	}
	return safe, nil
}

// MIMEFromFilename returns the MIME type for a filename's extension.
func MIMEFromFilename(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if m, ok := mimeFromExtension[ext]; ok {
		return m
	}
	return "application/octet-stream"
}

// ExtensionFromMIME returns a default file extension for a MIME type.
func ExtensionFromMIME(mime string) string {
	if ext, ok := extensionFromMIME[strings.ToLower(mime)]; ok {
		return ext
	}
	return "bin"
}

// PlaceholderForType returns the user-visible placeholder for non-text messages.
// Ported from whatsapp-bridge/index.js.
func PlaceholderForType(messageType string) string {
	switch strings.ToLower(messageType) {
	case "audio", "ptt":
		return "[voice note]"
	case "image":
		return "[image]"
	case "video":
		return "[video]"
	case "document":
		return "[document]"
	case "sticker":
		return "[sticker]"
	default:
		return "[media]"
	}
}

// MediaKey builds a deterministic S3 key for an inbound media file.
// Layout: tenants/{tenant_id}/inbound/{phone}/{timestamp}_{short_id}.{ext}
func MediaKey(tenantID, phone, shortID, ext string) string {
	return "tenants/" + tenantID + "/inbound/" + phone + "/" + shortID + "." + ext
}

// AssetKey builds a deterministic S3 key for an outbound asset.
// Layout: tenants/{tenant_id}/assets/{filename}
func AssetKey(tenantID, filename string) string {
	return "tenants/" + tenantID + "/assets/" + filename
}
