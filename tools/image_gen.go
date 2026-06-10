package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ImageGenFunc is a callback type for image generation
type ImageGenFunc func(ctx context.Context, prompt string, size int, provider string, output_path string) (string, error)

// ImageGenTool generates images via API
type ImageGenTool struct {
	imageGenFunc ImageGenFunc
}

// NewImageGenTool creates a new ImageGen tool with the given image generation callback
func NewImageGenTool(imageGenFunc ImageGenFunc) *ImageGenTool {
	return &ImageGenTool{
		imageGenFunc: imageGenFunc,
	}
}

func (t *ImageGenTool) Name() string {
	return "image_gen"
}

func (t *ImageGenTool) Description() string {
	return "Generate images via API. Supports OpenAI and Stability AI providers. Requires image generation to be configured with a callback function."
}

func (t *ImageGenTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Text prompt describing the image to generate",
			},
			"size": map[string]interface{}{
				"type":        "integer",
				"enum":        []int{256, 512, 1024},
				"description": "Image size in pixels (256, 512, or 1024)",
			},
			"output_path": map[string]interface{}{
				"type":        "string",
				"description": "File path where the generated image should be saved",
			},
			"provider": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"openai", "stability"},
				"description": "Image generation provider: openai or stability",
			},
		},
		"required": []string{"prompt", "size", "output_path", "provider"},
	}
}

func (t *ImageGenTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	// Check if image generation is configured
	if t.imageGenFunc == nil {
		return "image generation not configured", nil
	}

	var params struct {
		Prompt     string `json:"prompt"`
		Size       int    `json:"size"`
		OutputPath string `json:"output_path"`
		Provider   string `json:"provider"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate prompt
	if params.Prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	// Validate size
	if params.Size != 256 && params.Size != 512 && params.Size != 1024 {
		return "", fmt.Errorf("invalid size: %d. Must be one of: 256, 512, 1024", params.Size)
	}

	// Validate output_path
	if params.OutputPath == "" {
		return "", fmt.Errorf("output_path is required")
	}

	// Validate provider
	if params.Provider != "openai" && params.Provider != "stability" {
		return "", fmt.Errorf("unsupported provider: %s. Must be one of: openai, stability", params.Provider)
	}

	// Call image generation callback
	result, err := t.imageGenFunc(ctx, params.Prompt, params.Size, params.Provider, params.OutputPath)
	if err != nil {
		return "", fmt.Errorf("image generation failed: %w", err)
	}

	return result, nil
}