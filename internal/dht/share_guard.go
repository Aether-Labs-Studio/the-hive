package dht

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
)

const ceBinaryPayloadError = "community edition does not allow file or binary uploads"

var supportedDataURLPrefixes = []string{
	"data:image/",
	"data:audio/",
	"data:video/",
	"data:application/octet-stream",
	"data:application/pdf",
	"data:application/zip",
}

func rejectBinaryLikeContent(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}

	lower := strings.ToLower(trimmed)
	for _, prefix := range supportedDataURLPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return fmt.Errorf("security policy: %s", ceBinaryPayloadError)
		}
	}

	compact := stripWhitespace(trimmed)
	if len(compact) < 1024 {
		return nil
	}
	if len(compact)%4 != 0 || !looksLikeBase64(compact) {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(compact)
	if err != nil || len(decoded) < 256 {
		return nil
	}
	if hasKnownBinarySignature(decoded) || mostlyNonPrintable(decoded) {
		return fmt.Errorf("security policy: %s", ceBinaryPayloadError)
	}

	return nil
}

func stripWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, s)
}

func looksLikeBase64(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '+', r == '/', r == '=':
		default:
			return false
		}
	}
	return true
}

func hasKnownBinarySignature(decoded []byte) bool {
	signatures := [][]byte{
		{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
		{0xff, 0xd8, 0xff},
		{'G', 'I', 'F', '8'},
		{'R', 'I', 'F', 'F'},
		{'O', 'g', 'g', 'S'},
		{'I', 'D', '3'},
		{'%', 'P', 'D', 'F'},
		{'P', 'K', 0x03, 0x04},
	}
	for _, sig := range signatures {
		if bytes.HasPrefix(decoded, sig) {
			return true
		}
	}

	if len(decoded) > 12 && bytes.Equal(decoded[4:8], []byte("ftyp")) {
		return true
	}
	if len(decoded) > 12 && bytes.Equal(decoded[8:12], []byte("WEBP")) && bytes.HasPrefix(decoded, []byte("RIFF")) {
		return true
	}

	return false
}

func mostlyNonPrintable(decoded []byte) bool {
	var printable int
	for _, b := range decoded {
		if b == '\n' || b == '\r' || b == '\t' || (b >= 32 && b <= 126) {
			printable++
		}
	}
	return float64(printable)/float64(len(decoded)) < 0.85
}
