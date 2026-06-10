package sandbox

import (
	"fmt"
	"strings"
)

// ImageMap maps languages to Docker images
var ImageMap = map[string]string{
	"python":      "python:3.11-alpine",
	"python3":     "python:3.11-alpine",
	"node":        "node:20-alpine",
	"nodejs":      "node:20-alpine",
	"javascript":  "node:20-alpine",
	"js":          "node:20-alpine",
	"bash":        "alpine:latest",
	"sh":          "alpine:latest",
	"go":          "golang:1.23-alpine",
	"golang":      "golang:1.23-alpine",
	"ruby":        "ruby:3.2-alpine",
	"ruby3":       "ruby:3.2-alpine",
}

// ExtMap maps file extensions to languages
var ExtMap = map[string]string{
	".py":  "python",
	".js":  "nodejs",
	".sh":  "bash",
	".go":  "go",
	".rb":  "ruby",
}

// GetImageForLang returns the Docker image for a given language
func GetImageForLang(lang string) string {
	if lang == "" {
		return ""
	}

	// Normalize language name
	normalized := strings.ToLower(strings.TrimSpace(lang))

	// Direct lookup
	if img, ok := ImageMap[normalized]; ok {
		return img
	}

	// Try without version numbers
	if idx := strings.Index(normalized, "3"); idx > 0 {
		base := normalized[:idx]
		if img, ok := ImageMap[base]; ok {
			return img
		}
	}

	return ""
}

// GetCommandForLang returns the command to execute a file for a given language
func GetCommandForLang(lang, filePath string) []string {
	if lang == "" {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(lang))

	switch normalized {
	case "python", "python3":
		return []string{"python3", filePath}
	case "node", "nodejs", "javascript", "js":
		return []string{"node", filePath}
	case "bash", "sh":
		return []string{"sh", filePath}
	case "go", "golang":
		// For Go, we need to compile and run
		// This is a simplified approach; in practice you might want more sophisticated handling
		return []string{"sh", "-c", fmt.Sprintf("go run %s", filePath)}
	case "ruby", "ruby3":
		return []string{"ruby", filePath}
	default:
		return nil
	}
}

// GetFileExtension returns the file extension for a given language
func GetFileExtension(lang string) string {
	if lang == "" {
		return ""
	}

	normalized := strings.ToLower(strings.TrimSpace(lang))

	switch normalized {
	case "python", "python3":
		return ".py"
	case "node", "nodejs", "javascript", "js":
		return ".js"
	case "bash", "sh":
		return ".sh"
	case "go", "golang":
		return ".go"
	case "ruby", "ruby3":
		return ".rb"
	default:
		return ""
	}
}

// DetectLangFromExtension detects the language from a file extension
func DetectLangFromExtension(filePath string) string {
	ext := strings.ToLower(filePath)

	for suffix, lang := range ExtMap {
		if strings.HasSuffix(ext, suffix) {
			return lang
		}
	}

	return ""
}