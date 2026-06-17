package skill

import (
	"context"
	"log/slog"
	"sync"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
	"github.com/metabbe3/Kyoci-Agent/internal/skill/builtin"
)

// Registry wraps the kyoci.SkillRegistry to provide additional functionality
// for registering and managing built-in skills.
type Registry struct {
	registry *kyoci.SkillRegistry
	mu       sync.RWMutex
	logger   *slog.Logger
}

// NewRegistry creates a new skill registry.
func NewRegistry() *Registry {
	return &Registry{
		registry: kyoci.NewSkillRegistry(),
		logger:   slog.Default(),
	}
}

// Kyoci returns the underlying kyoci.SkillRegistry for use by the agent layer.
func (r *Registry) Kyoci() *kyoci.SkillRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registry
}

// Register adds a skill to the registry.
// Thread-safe: uses mutex to protect concurrent access.
func (r *Registry) Register(skill kyoci.Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Debug("registering skill", "name", skill.Name())
	return r.registry.Register(skill)
}

// Match finds a skill that can handle the given query.
// Thread-safe: uses mutex to protect concurrent access.
func (r *Registry) Match(query string) (kyoci.Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.registry.Match(query)
}

// List returns information about all registered skills.
// Thread-safe: uses mutex to protect concurrent access.
func (r *Registry) List() []kyoci.SkillInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.registry.List()
}

// RegisterBuiltin registers all built-in skills.
// This should be called during application initialization.
func (r *Registry) RegisterBuiltin() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("registering built-in skills")

	// Register all built-in skills
	builtinSkills := []kyoci.Skill{
		// Original six
		builtin.NewMathSkill(),
		builtin.NewTimeSkill(),
		builtin.NewHashSkill(),
		builtin.NewUUIDSkill(),
		builtin.NewEncodeSkill(),
		builtin.NewConvertSkill(),
		// Liquid Glass expansion — 14 new zero-AI fast paths
		builtin.NewColorSkill(),
		builtin.NewRegexSkill(),
		builtin.NewJSONFmtSkill(),
		builtin.NewSQLFmtSkill(),
		builtin.NewDiffSkill(),
		builtin.NewJWTSkill(),
		builtin.NewQRSkill(),
		builtin.NewPasswordSkill(),
		builtin.NewCharsetSkill(),
		builtin.NewCronSkill(),
		builtin.NewSubnetSkill(),
		builtin.NewLoremSkill(),
		builtin.NewMarkdownSkill(),
		builtin.NewEmojiInfoSkill(),

		// Catalog expansion (100+ skills) — grouped by file.

		// encoding.go — 12 skills
		builtin.NewBase64EncodeSkill(),
		builtin.NewBase64DecodeSkill(),
		builtin.NewBase32EncodeSkill(),
		builtin.NewBase32DecodeSkill(),
		builtin.NewURLEncodeSkill(),
		builtin.NewURLDecodeSkill(),
		builtin.NewHTMLEscapeSkill(),
		builtin.NewHTMLUnescapeSkill(),
		builtin.NewHexEncodeSkill(),
		builtin.NewHexDecodeSkill(),
		builtin.NewUnicodeEscapeSkill(),
		builtin.NewUnicodeUnescapeSkill(),

		// hashing.go — 13 skills
		builtin.NewMD5Skill(),
		builtin.NewSHA1Skill(),
		builtin.NewSHA256Skill(),
		builtin.NewSHA512Skill(),
		builtin.NewSHA3Skill(),
		builtin.NewCRC32Skill(),
		builtin.NewCRC64Skill(),
		builtin.NewHMACSHA256Skill(),
		builtin.NewHMACSHA512Skill(),
		builtin.NewBcryptHashSkill(),
		builtin.NewBcryptVerifySkill(),
		builtin.NewAESEncryptSkill(),
		builtin.NewAESDecryptSkill(),

		// security.go — 4 skills
		builtin.NewPasswordStrengthSkill(),
		builtin.NewSecretRedactSkill(),
		builtin.NewHashIdentifySkill(),
		builtin.NewCVEParseSkill(),

		// datafmt.go — 12 skills
		builtin.NewYAMLToJSONSkill(),
		builtin.NewJSONToYAMLSkill(),
		builtin.NewTOMLToJSONSkill(),
		builtin.NewJSONToTOMLSkill(),
		builtin.NewCSVToJSONSkill(),
		builtin.NewJSONToCSVSkill(),
		builtin.NewXMLToJSONSkill(),
		builtin.NewJSONToXMLSkill(),
		builtin.NewJSONMinifySkill(),
		builtin.NewJSONPrettySkill(),
		builtin.NewEnvToJSONSkill(),
		builtin.NewJSONToEnvSkill(),

		// text.go — 15 skills
		builtin.NewSlugifySkill(),
		builtin.NewCaseConvertSkill(),
		builtin.NewLevenshteinSkill(),
		builtin.NewCharCountSkill(),
		builtin.NewWordCountSkill(),
		builtin.NewLineCountSkill(),
		builtin.NewByteCountSkill(),
		builtin.NewTruncateSkill(),
		builtin.NewPadSkill(),
		builtin.NewReverseSkill(),
		builtin.NewSortLinesSkill(),
		builtin.NewDedupeLinesSkill(),
		builtin.NewIndentSkill(),
		builtin.NewDedentSkill(),
		builtin.NewRegexReplaceSkill(),

		// generators.go (text.go) — 10 skills
		builtin.NewUUIDV4Skill(),
		builtin.NewUUIDV7Skill(),
		builtin.NewNanoidSkill(),
		builtin.NewGUIDSkill(),
		builtin.NewRandomIntSkill(),
		builtin.NewRandomStringSkill(),
		builtin.NewRandomBytesSkill(),
		builtin.NewNonceSkill(),
		builtin.NewFakeNameSkill(),
		builtin.NewFakeEmailSkill(),

		// net.go — 9 skills
		builtin.NewIPValidateSkill(),
		builtin.NewIPInfoSkill(),
		builtin.NewMACLookupSkill(),
		builtin.NewPortCheckSkill(),
		builtin.NewURLParseSkill(),
		builtin.NewURLBuildSkill(),
		builtin.NewCIDRValidateSkill(),
		builtin.NewCIDRMergeSkill(),
		builtin.NewDNSLookupSkill(),

		// color_extended.go — 8 skills
		builtin.NewHexToRGBSkill(),
		builtin.NewRGBToHexSkill(),
		builtin.NewHexToHSLSkill(),
		builtin.NewHSLToHexSkill(),
		builtin.NewContrastRatioSkill(),
		builtin.NewColorBlendSkill(),
		builtin.NewPaletteAnalogousSkill(),
		builtin.NewPaletteComplementarySkill(),

		// math_extended.go — 12 skills
		builtin.NewStatsSkill(),
		builtin.NewGCDSkill(),
		builtin.NewLCMSkill(),
		builtin.NewIsPrimeSkill(),
		builtin.NewPrimeFactorsSkill(),
		builtin.NewFactorialSkill(),
		builtin.NewBaseConvertSkill(),
		builtin.NewRoundSigSkill(),
		builtin.NewUnitsConvertSkill(),
		builtin.NewCurrencyFormatSkill(),
		builtin.NewPercentageSkill(),
		builtin.NewRatioSimplifySkill(),

		// time_extended.go — 6 skills
		builtin.NewNowSkill(),
		builtin.NewTimeParseSkill(),
		builtin.NewTimeFormatSkill(),
		builtin.NewTimeDiffSkill(),
		builtin.NewCronNextSkill(),
		builtin.NewEpochConvertSkill(),

		// markdown_extended.go — 4 skills
		builtin.NewMarkdownOutlineSkill(),
		builtin.NewMarkdownTOCSkill(),
		builtin.NewMarkdownStripSkill(),
		builtin.NewMarkdownLinkExtractSkill(),
	}

	for _, skill := range builtinSkills {
		if err := r.registry.Register(skill); err != nil {
			r.logger.Error("failed to register built-in skill", "name", skill.Name(), "error", err)
			return err
		}
		r.logger.Info("built-in skill registered", "name", skill.Name())
	}

	return nil
}

// Execute executes a skill by name with the given query.
// Thread-safe: delegates to the underlying registry.
func (r *Registry) Execute(ctx context.Context, name, query string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.registry.Execute(ctx, name, query)
}