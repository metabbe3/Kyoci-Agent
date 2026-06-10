package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// PDFTool extracts text and information from PDF files using basic BT...ET marker parsing
type PDFTool struct{}

// NewPDFTool creates a new PDF tool
func NewPDFTool() *PDFTool {
	return &PDFTool{}
}

func (t *PDFTool) Name() string {
	return "pdf"
}

func (t *PDFTool) Description() string {
	return "Extract text or information from PDF files. Uses simple BT...ET (Begin Text...End Text) marker parsing for text extraction. Returns extracted text (truncated to 100KB) or file metadata. Works for most text-based PDFs."
}

func (t *PDFTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action to perform: read (extract text) or info (get file metadata)",
				"enum":        []string{"read", "info"},
			},
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the PDF file",
			},
		},
		"required": []string{"action", "file_path"},
	}
}

type pdfParams struct {
	Action   string `json:"action"`
	FilePath string `json:"file_path"`
}

func (t *PDFTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params pdfParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate required parameters
	if params.Action == "" {
		return "", fmt.Errorf("action is required")
	}
	if params.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}

	// Validate action
	if params.Action != "read" && params.Action != "info" {
		return "", fmt.Errorf("invalid action: %s (valid: read, info)", params.Action)
	}

	// Read PDF file
	data, err := os.ReadFile(params.FilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read PDF file: %w", err)
	}

	// Verify it's a PDF file
	if !t.isPDF(data) {
		return "", fmt.Errorf("file is not a valid PDF (missing %%PDF header)")
	}

	// Perform requested action
	switch params.Action {
	case "info":
		return t.extractInfo(data, params.FilePath)
	case "read":
		return t.extractText(data)
	default:
		return "", fmt.Errorf("unknown action: %s", params.Action)
	}
}

// isPDF checks if the data starts with the PDF magic number
func (t *PDFTool) isPDF(data []byte) bool {
	if len(data) < 5 {
		return false
	}
	return string(data[:5]) == "%PDF-"
}

// extractInfo extracts metadata from the PDF
func (t *PDFTool) extractInfo(data []byte, filePath string) (string, error) {
	pdfStr := string(data)

	// Get file size
	info := []string{
		fmt.Sprintf("File: %s", filePath),
		fmt.Sprintf("Size: %d bytes", len(data)),
	}

	// Extract PDF version
	versionRegex := regexp.MustCompile(`%PDF-(\d+\.\d+)`)
	if match := versionRegex.FindStringSubmatch(pdfStr); len(match) > 1 {
		info = append(info, fmt.Sprintf("PDF Version: %s", match[1]))
	}

	// Extract number of pages (common in /Count fields)
	countRegex := regexp.MustCompile(`/Type\s*/Page[^s]*/Count\s+(\d+)`)
	if match := countRegex.FindStringSubmatch(pdfStr); len(match) > 1 {
		info = append(info, fmt.Sprintf("Pages: %s", match[1]))
	}

	// Try to find page count in PageTree
	if len(info) == 3 { // Only have file, size, version
		pageCountRegex := regexp.MustCompile(`/Count\s+(\d+)`)
		matches := pageCountRegex.FindAllStringSubmatch(pdfStr, -1)
		// The last /Count before /Kids or /Root is likely the page count
		for i, match := range matches {
			if len(match) > 1 && strings.Contains(pdfStr[max(0, strings.Index(pdfStr, match[0])-200):strings.Index(pdfStr, match[0])+200], "/Kids") {
				info = append(info, fmt.Sprintf("Pages: %s", match[1]))
				break
			}
			if i == len(matches)-1 && len(match) > 1 {
				info = append(info, fmt.Sprintf("Pages: %s", match[1]))
			}
		}
	}

	// Extract title
	titleRegex := regexp.MustCompile(`/Title\s*\(([^)]*)\)`)
	if match := titleRegex.FindStringSubmatch(pdfStr); len(match) > 1 {
		info = append(info, fmt.Sprintf("Title: %s", match[1]))
	} else {
		// Try with <> delimiters
		titleRegex2 := regexp.MustCompile(`/Title\s*<([^>]*)>`)
		if match := titleRegex2.FindStringSubmatch(pdfStr); len(match) > 1 {
			info = append(info, fmt.Sprintf("Title: %s", match[1]))
		}
	}

	// Extract author
	authorRegex := regexp.MustCompile(`/Author\s*\(([^)]*)\)`)
	if match := authorRegex.FindStringSubmatch(pdfStr); len(match) > 1 {
		info = append(info, fmt.Sprintf("Author: %s", match[1]))
	}

	// Extract subject
	subjectRegex := regexp.MustCompile(`/Subject\s*\(([^)]*)\)`)
	if match := subjectRegex.FindStringSubmatch(pdfStr); len(match) > 1 {
		info = append(info, fmt.Sprintf("Subject: %s", match[1]))
	}

	// Extract creator
	creatorRegex := regexp.MustCompile(`/Creator\s*\(([^)]*)\)`)
	if match := creatorRegex.FindStringSubmatch(pdfStr); len(match) > 1 {
		info = append(info, fmt.Sprintf("Creator: %s", match[1]))
	}

	// Extract producer
	producerRegex := regexp.MustCompile(`/Producer\s*\(([^)]*)\)`)
	if match := producerRegex.FindStringSubmatch(pdfStr); len(match) > 1 {
		info = append(info, fmt.Sprintf("Producer: %s", match[1]))
	}

	// Extract creation date
	dateRegex := regexp.MustCompile(`/CreationDate\s*\(([^)]*)\)`)
	if match := dateRegex.FindStringSubmatch(pdfStr); len(match) > 1 {
		info = append(info, fmt.Sprintf("Created: %s", match[1]))
	}

	// Check if text extraction is likely possible
	btCount := strings.Count(pdfStr, "BT")
	info = append(info, fmt.Sprintf("Text blocks (BT...ET markers): %d", btCount))

	if btCount == 0 {
		info = append(info, "Note: This PDF may contain scanned images or use non-standard text encoding. Text extraction may return empty results.")
	}

	return strings.Join(info, "\n"), nil
}

// extractText extracts text content from PDF using BT...ET markers
func (t *PDFTool) extractText(data []byte) (string, error) {
	pdfStr := string(data)

	// Find all text blocks between BT and ET markers
	var textBlocks []string
	btPositions := t.findMatchingBTPairs(pdfStr)

	for _, pair := range btPositions {
		block := pdfStr[pair.bt+2 : pair.et] // Exclude BT and ET
		text := t.extractTextFromBlock(block)
		if text != "" {
			textBlocks = append(textBlocks, text)
		}
	}

	// Combine and clean text
	result := strings.Join(textBlocks, "\n")
	result = t.cleanText(result)

	// Truncate to 100KB
	maxSize := 100 * 1024
	if len(result) > maxSize {
		result = result[:maxSize] + "\n... (truncated to 100KB)"
	}

	if result == "" {
		return "No text found in PDF. The PDF may contain scanned images, encrypted content, or use non-standard text encoding.", nil
	}

	return result, nil
}

// btETPair represents a matching pair of BT and ET markers
type btETPair struct {
	bt int // position of BT marker
	et int // position of ET marker
}

// findMatchingBTPairs finds all matching BT...ET pairs in the PDF
func (t *PDFTool) findMatchingBTPairs(pdf string) []btETPair {
	var pairs []btETPair
	depth := 0
	currentBT := -1

	// Scan through the PDF
	for i := 0; i < len(pdf); i++ {
		// Look for BT (Begin Text)
		if i+1 < len(pdf) && pdf[i] == 'B' && pdf[i+1] == 'T' {
			// Check it's not part of a longer word
			before := byte(' ')
			if i > 0 {
				before = pdf[i-1]
			}
			after := byte(' ')
			if i+2 < len(pdf) {
				after = pdf[i+2]
			}
			// BT must be surrounded by whitespace or delimiters
			if (before == ' ' || before == '\n' || before == '\r' || before == '(' || before == '[' || before == '<' || before == '/') &&
				(after == ' ' || after == '\n' || after == '\r' || after == ')' || after == ']' || after == '>') {
				if depth == 0 {
					currentBT = i
				}
				depth++
				i++ // Skip the T
			}
		}

		// Look for ET (End Text)
		if i+1 < len(pdf) && pdf[i] == 'E' && pdf[i+1] == 'T' {
			// Check it's not part of a longer word
			before := byte(' ')
			if i > 0 {
				before = pdf[i-1]
			}
			after := byte(' ')
			if i+2 < len(pdf) {
				after = pdf[i+2]
			}
			// ET must be surrounded by whitespace or delimiters
			if (before == ' ' || before == '\n' || before == '\r' || before == ')' || before == ']') &&
				(after == ' ' || after == '\n' || after == '\r' || after == ')' || after == ']' || after == '>') {
				if depth > 0 {
					depth--
					if depth == 0 && currentBT >= 0 {
						pairs = append(pairs, btETPair{bt: currentBT, et: i})
						currentBT = -1
					}
				}
				i++ // Skip the T
			}
		}
	}

	return pairs
}

// extractTextFromBlock extracts text from a single BT...ET block
func (t *PDFTool) extractTextFromBlock(block string) string {
	var result strings.Builder

	// Parse Tj (show text) and TJ (show text with kerning) operations
	i := 0
	for i < len(block) {
		// Skip whitespace
		for i < len(block) && (block[i] == ' ' || block[i] == '\n' || block[i] == '\r' || block[i] == '\t') {
			i++
		}

		if i >= len(block) {
			break
		}

		// Look for text string in parentheses: (text)
		if block[i] == '(' {
			text, newPos := t.extractParenthesizedText(block, i)
			if text != "" {
				result.WriteString(text)
				result.WriteRune(' ')
			}
			i = newPos
			continue
		}

		// Look for text string in array: [(text1)(text2)]
		if block[i] == '[' {
			texts, newPos := t.extractArrayText(block, i)
			for _, text := range texts {
				if text != "" {
					result.WriteString(text)
					result.WriteRune(' ')
				}
			}
			i = newPos
			continue
		}

		// Look for hex string: <hex>
		if block[i] == '<' {
			text, newPos := t.extractHexText(block, i)
			if text != "" {
				result.WriteString(text)
				result.WriteRune(' ')
			}
			i = newPos
			continue
		}

		i++
	}

	return result.String()
}

// extractParenthesizedText extracts text from parentheses: (text)
func (t *PDFTool) extractParenthesizedText(block string, start int) (string, int) {
	if start >= len(block) || block[start] != '(' {
		return "", start
	}

	var result strings.Builder
	i := start + 1
	escaped := false

	for i < len(block) {
		ch := block[i]

		if escaped {
			// Handle escaped characters
			switch ch {
			case 'n':
				result.WriteRune('\n')
			case 'r':
				result.WriteRune('\r')
			case 't':
				result.WriteRune('\t')
			case 'b':
				result.WriteRune('\b')
			case 'f':
				result.WriteRune('\f')
			case '(':
				result.WriteRune('(')
			case ')':
				result.WriteRune(')')
			case '\\':
				result.WriteRune('\\')
			default:
				// Octal escape \nnn
				if ch >= '0' && ch <= '7' && i+2 < len(block) && block[i+1] >= '0' && block[i+1] <= '7' && block[i+2] >= '0' && block[i+2] <= '7' {
					octal := string([]byte{ch, block[i+1], block[i+2]})
					var code byte
					fmt.Sscanf(octal, "%o", &code)
					result.WriteByte(code)
					i += 2
								} else {
									result.WriteRune(rune(ch))
								}
			}
			escaped = false
		} else if ch == '\\' {
			escaped = true
		} else if ch == ')' {
			// End of string
			return result.String(), i + 1
		} else {
			result.WriteByte(ch)
		}
		i++
	}

	return result.String(), i
}

// extractArrayText extracts text from array notation: [(text1)(text2)]
func (t *PDFTool) extractArrayText(block string, start int) ([]string, int) {
	if start >= len(block) || block[start] != '[' {
		return nil, start
	}

	var texts []string
	i := start + 1
	depth := 1

	for i < len(block) && depth > 0 {
		if block[i] == '[' {
			depth++
		} else if block[i] == ']' {
			depth--
			if depth == 0 {
				return texts, i + 1
			}
		} else if block[i] == '(' && depth == 1 {
			text, newPos := t.extractParenthesizedText(block, i)
			texts = append(texts, text)
			i = newPos
			continue
		}
		i++
	}

	return texts, i
}

// extractHexText extracts text from hex string: <hex>
func (t *PDFTool) extractHexText(block string, start int) (string, int) {
	if start >= len(block) || block[start] != '<' {
		return "", start
	}

	i := start + 1
	var hexStr strings.Builder

	for i < len(block) && block[i] != '>' {
		if (block[i] >= '0' && block[i] <= '9') || (block[i] >= 'a' && block[i] <= 'f') || (block[i] >= 'A' && block[i] <= 'F') {
			hexStr.WriteByte(block[i])
		}
		i++
	}

	// Convert hex to bytes
	if i < len(block) && block[i] == '>' {
		hex := hexStr.String()
		var result strings.Builder
		for j := 0; j < len(hex); j += 2 {
			if j+1 < len(hex) {
				var b byte
				fmt.Sscanf(hex[j:j+2], "%02x", &b)
				result.WriteByte(b)
			}
		}
		return result.String(), i + 1
	}

	return "", i
}

// cleanText cleans and normalizes extracted text
func (t *PDFTool) cleanText(text string) string {
	// Remove excessive whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	// Remove common PDF artifacts
	text = regexp.MustCompile(`\b[\d\.,]+\s*\)`).ReplaceAllString(text, "") // Remove footnote references like (123)
	text = strings.ReplaceAll(text, "ﬁ", "fi")
	text = strings.ReplaceAll(text, "ﬂ", "fl")
	text = strings.ReplaceAll(text, "ﬀ", "ff")
	text = strings.ReplaceAll(text, "ﬃ", "ffi")
	text = strings.ReplaceAll(text, "ﬄ", "ffl")

	// Trim and normalize spaces
	text = strings.TrimSpace(text)

	return text
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}