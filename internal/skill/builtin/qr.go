package builtin

import (
	"context"
	"fmt"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// QRSkill receives QR code input and reports a placeholder until the
// go-qrcode dependency is added.
type QRSkill struct {
	*kyoci.BaseSkill
}

// NewQRSkill creates a new QR skill.
func NewQRSkill() *QRSkill {
	return &QRSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"qr",
			"Generate a QR code as ASCII art for a given string",
			[]string{"qr", "qr code", "qrcode"},
		),
	}
}

// Match returns true if the query mentions qr.
func (s *QRSkill) Match(query string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(query)), "qr")
}

// Execute reports the received input and explains the missing dependency.
func (s *QRSkill) Execute(ctx context.Context, query string) (string, error) {
	text := extractQRInput(query)
	if text == "" {
		return "", fmt.Errorf("no text provided to encode as QR")
	}
	return fmt.Sprintf(
		"QR input received: %s\nQR rendering requires adding github.com/skip2/go-qrcode to go.mod — this is a placeholder until then.",
		text,
	), nil
}

// extractQRInput strips command words and returns the text to encode.
func extractQRInput(query string) string {
	q := strings.TrimSpace(query)
	lower := strings.ToLower(q)

	prefixes := []string{"generate qr code", "generate qr", "qr code", "qrcode", "qr"}
	for _, p := range prefixes {
		if lower == p {
			return ""
		}
		if strings.HasPrefix(lower, p+" ") {
			return strings.TrimSpace(q[len(p):])
		}
	}
	return q
}
