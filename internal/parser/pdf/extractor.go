package pdf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// execCommandContext is a helper to create an exec.Cmd with context
func execCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// TextBlock represents a block of text with position info
type TextBlock struct {
	Text   string
	Page   int
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// ExtractedText holds all text extracted from a PDF
type ExtractedText struct {
	Pages      []PageText
	RawText    string
	Blocks     []TextBlock
	PageCount  int
}

// PageText holds text from a single page
type PageText struct {
	PageNum int
	Text    string
	Lines   []string
}

// Extractor handles PDF text extraction
type Extractor struct {
	conf *model.Configuration
}

// NewExtractor creates a new PDF text extractor
func NewExtractor() *Extractor {
	return &Extractor{
		conf: model.NewDefaultConfiguration(),
	}
}

// Extract extracts text from PDF content
// Note: pdfcpu's text extraction writes to files, so we use a temp directory
func (e *Extractor) Extract(ctx context.Context, r io.Reader) (*ExtractedText, error) {
	// Read all content into buffer
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF content: %w", err)
	}

	// Create reader from content
	reader := bytes.NewReader(content)

	// Get page count
	pageCount, err := api.PageCount(reader, e.conf)
	if err != nil {
		return nil, fmt.Errorf("failed to get page count: %w", err)
	}

	result := &ExtractedText{
		Pages:     make([]PageText, 0, pageCount),
		PageCount: pageCount,
	}

	// Create temp directory for extraction
	tmpDir, err := os.MkdirTemp("", "pdf-extract-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Reset reader
	reader.Reset(content)

	// Extract content to temp files
	err = api.ExtractContent(reader, tmpDir, "content", nil, e.conf)
	if err != nil {
		// Content extraction failed, try to read raw PDF structure
		reader.Reset(content)
		return e.extractFromContext(reader, pageCount)
	}

	// Read extracted content files
	var allText strings.Builder
	files, _ := os.ReadDir(tmpDir)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tmpDir, f.Name()))
		if err != nil {
			continue
		}
		// Extract readable text from content stream
		text := extractTextFromContentStream(string(data))
		if text != "" {
			allText.WriteString(text)
			allText.WriteString("\n")
		}
	}

	result.RawText = allText.String()
	if result.RawText != "" {
		result.Pages = append(result.Pages, PageText{
			PageNum: 1,
			Text:    result.RawText,
			Lines:   splitIntoLines(result.RawText),
		})
	}

	return result, nil
}

// extractFromContext tries to extract text from PDF context
func (e *Extractor) extractFromContext(reader *bytes.Reader, pageCount int) (*ExtractedText, error) {
	result := &ExtractedText{
		Pages:     make([]PageText, 0, pageCount),
		PageCount: pageCount,
	}

	// Read and validate PDF
	ctx, err := api.ReadAndValidate(reader, e.conf)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}

	var allText strings.Builder

	// Try to extract text from each page's content stream
	for i := 1; i <= pageCount; i++ {
		pageReader, err := api.ExtractPage(ctx, i)
		if err != nil {
			continue
		}
		pageContent, err := io.ReadAll(pageReader)
		if err != nil {
			continue
		}
		text := extractTextFromContentStream(string(pageContent))
		if text != "" {
			result.Pages = append(result.Pages, PageText{
				PageNum: i,
				Text:    text,
				Lines:   splitIntoLines(text),
			})
			allText.WriteString(text)
			allText.WriteString("\n")
		}
	}

	result.RawText = allText.String()
	return result, nil
}

// extractTextFromContentStream extracts readable text from PDF content stream
func extractTextFromContentStream(content string) string {
	var result strings.Builder

	// PDF text operators: Tj, TJ, ' "
	// Look for text between ( ) or < > for hex strings

	// Extract strings in parentheses (PDF literal strings)
	reParens := regexp.MustCompile(`\(([^)]*)\)`)
	matches := reParens.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) > 1 {
			text := unescapePDFString(m[1])
			if isPrintableText(text) {
				result.WriteString(text)
				result.WriteString(" ")
			}
		}
	}

	// Extract hex strings
	reHex := regexp.MustCompile(`<([0-9A-Fa-f]+)>`)
	hexMatches := reHex.FindAllStringSubmatch(content, -1)
	for _, m := range hexMatches {
		if len(m) > 1 {
			text := hexToString(m[1])
			if isPrintableText(text) {
				result.WriteString(text)
				result.WriteString(" ")
			}
		}
	}

	return strings.TrimSpace(result.String())
}

// unescapePDFString handles PDF string escape sequences
func unescapePDFString(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\r", "\r")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\(", "(")
	s = strings.ReplaceAll(s, "\\)", ")")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// hexToString converts hex string to text
func hexToString(hex string) string {
	var result []byte
	for i := 0; i+1 < len(hex); i += 2 {
		var b byte
		fmt.Sscanf(hex[i:i+2], "%02x", &b)
		result = append(result, b)
	}
	return string(result)
}

// isPrintableText checks if string contains printable text
func isPrintableText(s string) bool {
	if len(s) == 0 {
		return false
	}
	printable := 0
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == ' ' || r == '.' ||
			r == ',' || r == ':' || r == '-' || r == '/' ||
			(r >= 0x00C0 && r <= 0x024F) || // Latin Extended
			(r >= 0x1E00 && r <= 0x1EFF) { // Vietnamese
			printable++
		}
	}
	return float64(printable)/float64(len(s)) > 0.5
}

// ExtractBytes extracts text from PDF bytes
func (e *Extractor) ExtractBytes(ctx context.Context, data []byte) (*ExtractedText, error) {
	return e.Extract(ctx, bytes.NewReader(data))
}

// ExtractWithPositions extracts text with position information
// This is more expensive but useful for template matching
func (e *Extractor) ExtractWithPositions(ctx context.Context, r io.Reader) (*ExtractedText, error) {
	// For basic implementation, we use the standard extraction
	// Position extraction would require more advanced PDF parsing
	return e.Extract(ctx, r)
}

// FindPattern searches for a regex pattern in extracted text
func (et *ExtractedText) FindPattern(pattern string) ([]string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	matches := re.FindAllString(et.RawText, -1)
	return matches, nil
}

// FindNear finds text near a label (useful for key-value extraction)
func (et *ExtractedText) FindNear(label string, maxDistance int) string {
	lines := strings.Split(et.RawText, "\n")

	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), strings.ToLower(label)) {
			// Check same line for value (after colon or label)
			if idx := strings.Index(line, ":"); idx >= 0 {
				value := strings.TrimSpace(line[idx+1:])
				if value != "" {
					return value
				}
			}

			// Check next few lines
			for j := 1; j <= maxDistance && i+j < len(lines); j++ {
				value := strings.TrimSpace(lines[i+j])
				if value != "" && !isLabel(value) {
					return value
				}
			}
		}
	}

	return ""
}

// GetLine returns a specific line from the extracted text
func (et *ExtractedText) GetLine(lineNum int) string {
	lines := strings.Split(et.RawText, "\n")
	if lineNum >= 0 && lineNum < len(lines) {
		return strings.TrimSpace(lines[lineNum])
	}
	return ""
}

// GetLines returns all lines from the extracted text
func (et *ExtractedText) GetLines() []string {
	return splitIntoLines(et.RawText)
}

// Helper functions

func splitIntoLines(text string) []string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

func isLabel(s string) bool {
	// Check if string looks like a label (ends with colon, common label patterns)
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, ":") {
		return true
	}

	// Common label patterns
	labels := []string{
		"mã số thuế", "tax id", "taxid",
		"số hóa đơn", "invoice no", "invoice number",
		"ngày", "date",
		"tên", "name",
		"địa chỉ", "address",
	}

	lower := strings.ToLower(s)
	for _, label := range labels {
		if strings.Contains(lower, label) && len(s) < 50 {
			return true
		}
	}

	return false
}

// ConvertToImages converts PDF bytes to PNG images using pdftoppm
// Returns a slice of PNG image bytes, one per page
func (e *Extractor) ConvertToImages(ctx context.Context, pdfData []byte) ([][]byte, error) {
	// Create temp directory for PDF and images
	tmpDir, err := os.MkdirTemp("", "pdf-images-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write PDF to temp file
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, pdfData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write temp PDF: %w", err)
	}

	// Convert PDF to PNG using pdftoppm
	outputPrefix := filepath.Join(tmpDir, "page")
	if err := convertPDFToImages(ctx, pdfPath, outputPrefix); err != nil {
		return nil, fmt.Errorf("failed to convert PDF to images: %w", err)
	}

	// Read generated images
	var images [][]byte
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp dir: %w", err)
	}

	for _, f := range files {
		name := f.Name()
		if f.IsDir() || (!strings.HasSuffix(name, ".png") && !strings.HasSuffix(name, ".jpg") && !strings.HasSuffix(name, ".jpeg")) {
			continue
		}
		imgPath := filepath.Join(tmpDir, name)
		imgData, err := os.ReadFile(imgPath)
		if err != nil {
			continue
		}
		images = append(images, imgData)
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("no images generated from PDF")
	}

	return images, nil
}

// convertPDFToImages runs pdftoppm to convert PDF to JPEG images
// Uses 100 DPI and JPEG compression to reduce file size and token consumption
func convertPDFToImages(ctx context.Context, pdfPath, outputPrefix string) error {
	// Try pdftoppm first (from poppler)
	// -jpeg: Use JPEG format for smaller file size
	// -r 100: 100 DPI is sufficient for invoice text recognition
	// -jpegopt quality=80: Good quality/size balance
	cmd := execCommandContext(ctx, "pdftoppm", "-jpeg", "-r", "100", "-jpegopt", "quality=80", pdfPath, outputPrefix)
	if err := cmd.Run(); err != nil {
		// Try convert from ImageMagick as fallback
		cmd = execCommandContext(ctx, "convert", "-density", "100", "-quality", "80", pdfPath, outputPrefix+".jpg")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("pdftoppm and convert both failed: %w", err)
		}
	}
	return nil
}
