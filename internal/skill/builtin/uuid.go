package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// UUIDSkill handles UUID generation.
type UUIDSkill struct {
	*kyoci.BaseSkill
}

// NewUUIDSkill creates a new UUID skill.
func NewUUIDSkill() *UUIDSkill {
	return &UUIDSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"uuid",
			"Generates a random UUID (Universally Unique Identifier)",
			[]string{"uuid", "generate uuid", "new uuid", "random uuid", "guid"},
		),
	}
}

// Match checks if the query is asking for a UUID.
func (s *UUIDSkill) Match(query string) bool {
	queryLower := strings.ToLower(query)
	for _, keyword := range []string{"uuid", "guid", "generate uuid", "new uuid"} {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}

// Execute generates and returns a UUID.
func (s *UUIDSkill) Execute(ctx context.Context, query string) (string, error) {
	queryLower := strings.ToLower(query)

	// Determine format
	if strings.Contains(queryLower, "no dash") || strings.Contains(queryLower, "no hyphen") {
		// UUID without dashes
		u := uuid.New()
		return fmt.Sprintf("UUID: %s", strings.ReplaceAll(u.String(), "-", "")), nil
	}

	// Default: standard UUID format
	u := uuid.New()
	return fmt.Sprintf("UUID: %s", u.String()), nil
}