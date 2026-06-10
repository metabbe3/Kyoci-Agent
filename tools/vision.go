package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// VisionFunc is a callback type for vision analysis
type VisionFunc func(ctx context.Context, imagePath string, question string) (string, error)

// VisionTool analyzes images from file paths or URLs
type VisionTool struct {
	visionFunc VisionFunc
}

// NewVisionTool creates a new Vision tool with the given vision callback
func NewVisionTool(visionFunc VisionFunc) *VisionTool {
	return &VisionTool{
		visionFunc: visionFunc,
	}
}

func (t *VisionTool) Name() string {
	return "vision"
}

func (t *VisionTool) Description() string {
	return "Analyze images and answer questions about their content. Provide either a local file path (image_path) or a URL (image_url) along with a question about the image."
}

func (t *VisionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"image_path": map[string]interface{}{
				"type":        "string",
				"description": "Local file path to the image to analyze",
			},
			"image_url": map[string]interface{}{
				"type":        "string",
				"description": "URL of the image to analyze (will be downloaded to a temporary file)",
			},
			"question": map[string]interface{}{
				"type":        "string",
				"description": "Question or prompt about the image (e.g., 'Describe what you see', 'What is in this image?', 'Is there a cat in this photo?')",
			},
		},
		"required": []string{"question"},
	}
}

func (t *VisionTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	// Check if vision is configured
	if t.visionFunc == nil {
		return "vision not configured", nil
	}

	var params struct {
		ImagePath string `json:"image_path"`
		ImageURL  string `json:"image_url"`
		Question  string `json:"question"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate question
	if params.Question == "" {
		return "", fmt.Errorf("question is required")
	}

	// Determine image source
	imagePath := ""

	switch {
	case params.ImagePath != "" && params.ImageURL != "":
		return "", fmt.Errorf("provide either image_path or image_url, not both")
	case params.ImagePath != "":
		imagePath = params.ImagePath
	case params.ImageURL != "":
		// Download image from URL to temp file
		downloadedPath, err := downloadImageToTemp(ctx, params.ImageURL)
		if err != nil {
			return "", fmt.Errorf("failed to download image: %w", err)
		}
		imagePath = downloadedPath
		defer os.Remove(downloadedPath)
	default:
		return "", fmt.Errorf("provide either image_path or image_url")
	}

	// Validate image file exists
	if _, err := os.Stat(imagePath); err != nil {
		return "", fmt.Errorf("image file not found: %w", err)
	}

	// Call vision callback
	result, err := t.visionFunc(ctx, imagePath, params.Question)
	if err != nil {
		return "", fmt.Errorf("vision analysis failed: %w", err)
	}

	return result, nil
}

// downloadImageToTemp downloads an image from a URL to a temporary file
func downloadImageToTemp(ctx context.Context, url string) (string, error) {
	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set reasonable timeout for download
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Create temp file
	ext := filepath.Ext(url)
	if ext == "" {
		ext = ".img"
	}
	tmpFile, err := os.CreateTemp("", "vision-download-*"+ext)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()
	tmpPath := tmpFile.Name()

	// Copy downloaded data to temp file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	return tmpPath, nil
}