package processor

import (
	"context"
	"fmt"
	"io"

	"github.com/rezonia/invoice-processor/internal/llm"
	"github.com/rezonia/invoice-processor/internal/model"
	"github.com/rezonia/invoice-processor/internal/parser/pdf"
	"github.com/rezonia/invoice-processor/internal/parser/xml"
)

// ExtractionMethod indicates how the invoice was extracted
type ExtractionMethod string

const (
	MethodXML       ExtractionMethod = "xml"
	MethodLLMText   ExtractionMethod = "llm_text"
	MethodLLMVision ExtractionMethod = "llm_vision"
)

// Result represents the extraction result with metadata
type Result struct {
	Invoice    *model.Invoice   `json:"invoice"`
	Method     ExtractionMethod `json:"method"`
	Confidence float64          `json:"confidence"`
	Warnings   []string         `json:"warnings,omitempty"`
	Error      error            `json:"-"`
}

// Pipeline orchestrates the hybrid extraction process
type Pipeline struct {
	xmlRegistry  *xml.Registry
	pdfExtractor *pdf.Extractor
	llmExtractor *llm.Extractor
}

// PipelineOption configures the pipeline
type PipelineOption func(*Pipeline)

// WithLLMExtractor sets the LLM extractor for PDF/image processing
func WithLLMExtractor(extractor *llm.Extractor) PipelineOption {
	return func(p *Pipeline) {
		p.llmExtractor = extractor
	}
}

// NewPipeline creates a new extraction pipeline
func NewPipeline(opts ...PipelineOption) *Pipeline {
	p := &Pipeline{
		xmlRegistry:  xml.NewRegistry(),
		pdfExtractor: pdf.NewExtractor(),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// ProcessXML processes an XML invoice from a reader
func (p *Pipeline) ProcessXML(ctx context.Context, r io.Reader) *Result {
	data, err := io.ReadAll(r)
	if err != nil {
		return &Result{
			Error: fmt.Errorf("failed to read XML: %w", err),
		}
	}
	return p.ProcessXMLBytes(ctx, data)
}

// ProcessXMLBytes processes XML invoice from bytes
func (p *Pipeline) ProcessXMLBytes(ctx context.Context, data []byte) *Result {
	inv, err := p.xmlRegistry.Parse(ctx, data)
	if err != nil {
		return &Result{
			Error: fmt.Errorf("XML parsing failed: %w", err),
		}
	}

	return &Result{
		Invoice:    inv,
		Method:     MethodXML,
		Confidence: 1.0, // XML is deterministic
	}
}

// ProcessPDF processes a PDF invoice using LLM extraction
func (p *Pipeline) ProcessPDF(ctx context.Context, r io.Reader, imageData []byte, mimeType string) *Result {
	if p.llmExtractor == nil {
		return &Result{
			Error: fmt.Errorf("LLM extractor not configured - required for PDF processing"),
		}
	}

	// Read PDF data
	var pdfData []byte
	var err error

	if r != nil {
		pdfData, err = io.ReadAll(r)
		if err != nil {
			return &Result{
				Error: fmt.Errorf("failed to read PDF: %w", err),
			}
		}
	} else if len(imageData) > 0 {
		pdfData = imageData
	} else {
		return &Result{
			Error: fmt.Errorf("no PDF data provided"),
		}
	}

	// Step 1: Try LLM text extraction (extract text from PDF, then use LLM)
	textResult := p.tryLLMTextExtraction(ctx, pdfData)
	if textResult.Invoice != nil && textResult.Error == nil {
		return textResult
	}

	// Step 2: Try LLM vision extraction as fallback
	visionResult := p.tryLLMVisionExtraction(ctx, pdfData, mimeType)
	if visionResult.Invoice != nil {
		return visionResult
	}

	// Return error with context from both attempts
	warnings := textResult.Warnings
	if visionResult.Error != nil {
		warnings = append(warnings, visionResult.Warnings...)
	}

	if visionResult.Error != nil {
		return &Result{
			Error:    fmt.Errorf("PDF extraction failed (text: %v, vision: %v)", textResult.Error, visionResult.Error),
			Warnings: warnings,
		}
	}

	if textResult.Error != nil {
		return &Result{
			Error:    fmt.Errorf("PDF extraction failed: %w", textResult.Error),
			Warnings: textResult.Warnings,
		}
	}

	return &Result{
		Error: fmt.Errorf("PDF extraction failed"),
	}
}

// ProcessImage processes an image invoice using LLM vision
func (p *Pipeline) ProcessImage(ctx context.Context, imageData []byte, mimeType string) *Result {
	if p.llmExtractor == nil {
		return &Result{
			Error: fmt.Errorf("LLM extractor not configured"),
		}
	}

	return p.tryLLMVisionExtraction(ctx, imageData, mimeType)
}

func (p *Pipeline) tryLLMTextExtraction(ctx context.Context, pdfData []byte) *Result {
	// Extract text from PDF
	extracted, err := p.pdfExtractor.ExtractBytes(ctx, pdfData)
	if err != nil {
		return &Result{
			Error:    err,
			Warnings: []string{fmt.Sprintf("PDF text extraction failed: %v", err)},
		}
	}

	if extracted.RawText == "" {
		return &Result{
			Error:    fmt.Errorf("no text extracted from PDF"),
			Warnings: []string{"PDF contains no extractable text"},
		}
	}

	// Use LLM to extract from text
	invoice, err := p.llmExtractor.ExtractFromOCRText(ctx, extracted.RawText)
	if err != nil {
		return &Result{
			Error:    err,
			Warnings: []string{fmt.Sprintf("LLM text extraction failed: %v", err)},
		}
	}

	return &Result{
		Invoice:    invoice,
		Method:     MethodLLMText,
		Confidence: 0.85, // LLM text extraction generally reliable
	}
}

func (p *Pipeline) tryLLMVisionExtraction(ctx context.Context, data []byte, mimeType string) *Result {
	var imageData []byte
	var imageMimeType string

	// If data is PDF, convert to image first
	if mimeType == "application/pdf" || (len(data) >= 4 && string(data[:4]) == "%PDF") {
		images, err := p.pdfExtractor.ConvertToImages(ctx, data)
		if err != nil {
			return &Result{
				Error:    fmt.Errorf("failed to convert PDF to images: %w", err),
				Warnings: []string{fmt.Sprintf("PDF to image conversion failed: %v", err)},
			}
		}
		// Use first page for vision extraction
		imageData = images[0]
		// Detect image format from magic bytes
		imageMimeType = detectImageMimeType(imageData)
	} else {
		imageData = data
		imageMimeType = mimeType
	}

	invoice, err := p.llmExtractor.ExtractFromImage(ctx, imageData, imageMimeType)
	if err != nil {
		return &Result{
			Error:    err,
			Warnings: []string{fmt.Sprintf("LLM vision extraction failed: %v", err)},
		}
	}

	return &Result{
		Invoice:    invoice,
		Method:     MethodLLMVision,
		Confidence: 0.80, // Vision slightly less reliable than text
	}
}

// detectImageMimeType detects the MIME type of image data from magic bytes
func detectImageMimeType(data []byte) string {
	if len(data) >= 3 {
		// JPEG: FF D8 FF
		if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
			return "image/jpeg"
		}
	}
	if len(data) >= 4 {
		// PNG: 89 50 4E 47
		if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
			return "image/png"
		}
	}
	// Default to JPEG since we now generate JPEG by default
	return "image/jpeg"
}

// DetectFormat detects the invoice format from file content
func DetectFormat(data []byte) Format {
	if len(data) == 0 {
		return FormatUnknown
	}

	// Check for XML declaration or common XML patterns
	if len(data) > 5 {
		header := string(data[:min(100, len(data))])
		if header[0] == '<' || contains(header, "<?xml") {
			return FormatXML
		}
	}

	// Check for PDF magic number
	if len(data) >= 4 && string(data[:4]) == "%PDF" {
		return FormatPDF
	}

	// Check for common image formats
	if len(data) >= 8 {
		// PNG
		if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
			return FormatImage
		}
		// JPEG
		if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
			return FormatImage
		}
		// TIFF (little-endian)
		if data[0] == 0x49 && data[1] == 0x49 && data[2] == 0x2A && data[3] == 0x00 {
			return FormatImage
		}
		// TIFF (big-endian)
		if data[0] == 0x4D && data[1] == 0x4D && data[2] == 0x00 && data[3] == 0x2A {
			return FormatImage
		}
	}

	return FormatUnknown
}

// Format represents the invoice file format
type Format int

const (
	FormatUnknown Format = iota
	FormatXML
	FormatPDF
	FormatImage
)

func (f Format) String() string {
	switch f {
	case FormatXML:
		return "xml"
	case FormatPDF:
		return "pdf"
	case FormatImage:
		return "image"
	default:
		return "unknown"
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
