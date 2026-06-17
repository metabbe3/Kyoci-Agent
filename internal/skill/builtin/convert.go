package builtin

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ConvertSkill handles unit conversions.
type ConvertSkill struct {
	*kyoci.BaseSkill
	pattern *regexp.Regexp
}

// NewConvertSkill creates a new unit conversion skill.
func NewConvertSkill() *ConvertSkill {
	skill := &ConvertSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"convert",
			"Converts between units (temperature, length, weight, storage)",
			[]string{"convert", "to", "fahrenheit", "celsius", "kelvin", "mile", "km", "km to miles", "kg", "lb", "gb", "mb", "tb"},
		),
	}
	// Pattern: "convert X from to Y" or "X unit1 to unit2"
	// Order longer units first to avoid partial matches
	skill.pattern = regexp.MustCompile(`(?i)(?:convert\s+)?(\d+(?:\.\d+)?)\s*(fahrenheit|celsius|kelvin|°f|°c|°k|kilometer|kilometers|meter|meters|kilogram|kilograms|pound|pounds|ounce|ounces|gigabyte|gigabytes|megabyte|megabytes|terabyte|terabytes|km|mi|ft|in|kg|lb|oz|gb|mb|tb|f|c|k|m)\s+to\s+(fahrenheit|celsius|kelvin|°f|°c|°k|kilometer|kilometers|meter|meters|kilogram|kilograms|pound|pounds|ounce|ounces|gigabyte|gigabytes|megabyte|megabytes|terabyte|terabytes|km|mi|ft|in|kg|lb|oz|gb|mb|tb|f|c|k|m)`)
	return skill
}

// Match checks if the query is asking for unit conversion.
func (s *ConvertSkill) Match(query string) bool {
	queryLower := strings.ToLower(query)
	if strings.Contains(queryLower, "convert") && strings.Contains(queryLower, "to") {
		return true
	}
	return s.pattern.MatchString(query)
}

// Execute performs the unit conversion.
func (s *ConvertSkill) Execute(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)

	// Use regex pattern to parse
	match := s.pattern.FindStringSubmatch(query)
	if match == nil {
		return "", fmt.Errorf("could not parse conversion query: %s", query)
	}

	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return "", fmt.Errorf("invalid number: %s", match[1])
	}

	from := strings.ToLower(match[2])
	to := strings.ToLower(match[3])

	result, err := s.convert(value, from, to)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%.2f %s = %.2f %s", value, from, result, to), nil
}

// convert performs the actual conversion.
func (s *ConvertSkill) convert(value float64, from, to string) (float64, error) {
	from = s.normalizeUnit(from)
	to = s.normalizeUnit(to)

	// Temperature conversions
	if s.isTemperature(from) && s.isTemperature(to) {
		celsius := s.toCelsius(value, from)
		return s.fromCelsius(celsius, to), nil
	}

	// Length conversions
	if s.isLength(from) && s.isLength(to) {
		meters := s.toMeters(value, from)
		return s.fromMeters(meters, to), nil
	}

	// Weight conversions
	if s.isWeight(from) && s.isWeight(to) {
		kg := s.toKg(value, from)
		return s.fromKg(kg, to), nil
	}

	// Storage conversions
	if s.isStorage(from) && s.isStorage(to) {
		bytes := s.toBytes(value, from)
		return s.fromBytes(bytes, to), nil
	}

	return 0, fmt.Errorf("incompatible units: cannot convert %s to %s", from, to)
}

// normalizeUnit normalizes unit strings.
func (s *ConvertSkill) normalizeUnit(unit string) string {
	unit = strings.ToLower(strings.TrimSpace(unit))
	aliases := map[string]string{
		"fahrenheit": "f", "f": "f", "°f": "f",
		"celsius": "c", "c": "c", "°c": "c",
		"kelvin": "k", "k": "k", "°k": "k",
		"meter": "m", "meters": "m", "m": "m",
		"kilometer": "km", "kilometers": "km", "km": "km",
		"mile": "mi", "miles": "mi", "mi": "mi",
		"feet": "ft", "foot": "ft", "ft": "ft",
		"inch": "in", "inches": "in", "in": "in",
		"kilogram": "kg", "kilograms": "kg", "kg": "kg",
		"pound": "lb", "pounds": "lb", "lbs": "lb", "lb": "lb",
		"ounce": "oz", "ounces": "oz", "oz": "oz",
		"gigabyte": "gb", "gigabytes": "gb", "gb": "gb",
		"megabyte": "mb", "megabytes": "mb", "mb": "mb",
		"terabyte": "tb", "terabytes": "tb", "tb": "tb",
	}
	if normalized, ok := aliases[unit]; ok {
		return normalized
	}
	return unit
}

// Temperature functions
func (s *ConvertSkill) isTemperature(unit string) bool {
	return unit == "f" || unit == "c" || unit == "k"
}

func (s *ConvertSkill) toCelsius(value float64, unit string) float64 {
	switch unit {
	case "f":
		return (value - 32) * 5 / 9
	case "k":
		return value - 273.15
	default: // c
		return value
	}
}

func (s *ConvertSkill) fromCelsius(value float64, unit string) float64 {
	switch unit {
	case "f":
		return value*9/5 + 32
	case "k":
		return value + 273.15
	default: // c
		return value
	}
}

// Length functions
func (s *ConvertSkill) isLength(unit string) bool {
	return unit == "m" || unit == "km" || unit == "mi" || unit == "ft" || unit == "in"
}

func (s *ConvertSkill) toMeters(value float64, unit string) float64 {
	switch unit {
	case "km":
		return value * 1000
	case "mi":
		return value * 1609.34
	case "ft":
		return value * 0.3048
	case "in":
		return value * 0.0254
	default: // m
		return value
	}
}

func (s *ConvertSkill) fromMeters(value float64, unit string) float64 {
	switch unit {
	case "km":
		return value / 1000
	case "mi":
		return value / 1609.34
	case "ft":
		return value / 0.3048
	case "in":
		return value / 0.0254
	default: // m
		return value
	}
}

// Weight functions
func (s *ConvertSkill) isWeight(unit string) bool {
	return unit == "kg" || unit == "lb" || unit == "oz"
}

func (s *ConvertSkill) toKg(value float64, unit string) float64 {
	switch unit {
	case "lb":
		return value * 0.453592
	case "oz":
		return value * 0.0283495
	default: // kg
		return value
	}
}

func (s *ConvertSkill) fromKg(value float64, unit string) float64 {
	switch unit {
	case "lb":
		return value / 0.453592
	case "oz":
		return value / 0.0283495
	default: // kg
		return value
	}
}

// Storage functions
func (s *ConvertSkill) isStorage(unit string) bool {
	return unit == "gb" || unit == "mb" || unit == "tb" || unit == "kb" || unit == "b"
}

func (s *ConvertSkill) toBytes(value float64, unit string) float64 {
	switch unit {
	case "tb":
		return value * 1024 * 1024 * 1024 * 1024
	case "gb":
		return value * 1024 * 1024 * 1024
	case "mb":
		return value * 1024 * 1024
	case "kb":
		return value * 1024
	default: // b
		return value
	}
}

func (s *ConvertSkill) fromBytes(value float64, unit string) float64 {
	switch unit {
	case "tb":
		return value / (1024 * 1024 * 1024 * 1024)
	case "gb":
		return value / (1024 * 1024 * 1024)
	case "mb":
		return value / (1024 * 1024)
	case "kb":
		return value / 1024
	default: // b
		return value
	}
}