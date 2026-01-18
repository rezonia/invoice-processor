package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rezonia/invoice-processor/internal/model"
)

// Extractor uses LLM to extract invoice data
type Extractor struct {
	client      *Client
	textModel   string
	visionModel string
}

// ExtractorOption configures the extractor
type ExtractorOption func(*Extractor)

// WithModel sets the model to use for text extraction
func WithModel(model string) ExtractorOption {
	return func(e *Extractor) {
		e.textModel = model
	}
}

// WithTextModel sets the model to use for text extraction (alias for WithModel)
func WithTextModel(model string) ExtractorOption {
	return func(e *Extractor) {
		e.textModel = model
	}
}

// WithVisionModel sets the model to use for vision/image extraction
func WithVisionModel(model string) ExtractorOption {
	return func(e *Extractor) {
		e.visionModel = model
	}
}

// NewExtractor creates a new LLM-based extractor
func NewExtractor(client *Client, opts ...ExtractorOption) *Extractor {
	e := &Extractor{
		client:      client,
		textModel:   ModelClaude35Sonnet, // Default to Claude for best results
		visionModel: ModelClaude35Sonnet, // Default to Claude for vision
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// ExtractFromText extracts invoice data from OCR text
func (e *Extractor) ExtractFromText(ctx context.Context, text string) (*model.Invoice, error) {
	prompt := fmt.Sprintf(UserPromptTextExtraction, text)

	response, err := e.client.ChatText(ctx, e.textModel, SystemPromptInvoiceExtractor, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	return e.parseResponse(response)
}

// ExtractFromImage extracts invoice data directly from an image
func (e *Extractor) ExtractFromImage(ctx context.Context, imageData []byte, mimeType string) (*model.Invoice, error) {
	response, err := e.client.ChatWithImage(ctx, e.visionModel, SystemPromptInvoiceExtractor, UserPromptImageExtraction, imageData, mimeType)
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	return e.parseResponse(response)
}

// ExtractFromOCRText extracts invoice data from potentially noisy OCR text
func (e *Extractor) ExtractFromOCRText(ctx context.Context, ocrText string) (*model.Invoice, error) {
	prompt := fmt.Sprintf(UserPromptOCRCorrection, ocrText)

	response, err := e.client.ChatText(ctx, e.textModel, SystemPromptInvoiceExtractor, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	return e.parseResponse(response)
}

// LLMResponse represents the JSON structure returned by LLM
type LLMResponse struct {
	InvoiceNumber  string        `json:"invoice_number"`
	Series         string        `json:"series"`
	Date           string        `json:"date"`
	Type           string        `json:"type"`
	Seller         LLMParty      `json:"seller"`
	Buyer          LLMParty      `json:"buyer"`
	Items          []LLMLineItem `json:"items"`
	Subtotal       json.Number   `json:"subtotal"`
	TotalDiscount  json.Number   `json:"total_discount"`
	TotalVAT       json.Number   `json:"total_vat"`
	TotalAmount    json.Number   `json:"total_amount"`
	Currency       string        `json:"currency"`
	PaymentMethod  string        `json:"payment_method"`
	Notes          string        `json:"notes"`
	// Receipt-specific fields
	DocumentType   string      `json:"document_type"`
	ReceiptNumber  string      `json:"receipt_number"`
	Cashier        string      `json:"cashier"`
	TerminalID     string      `json:"terminal_id"`
	Time           string      `json:"time"`
	AmountTendered json.Number `json:"amount_tendered"`
	Change         json.Number `json:"change"`
}

// LLMParty represents a party in the LLM response
type LLMParty struct {
	Name        string `json:"name"`
	TaxID       string `json:"tax_id"`
	Address     string `json:"address"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	BankAccount string `json:"bank_account"`
	BankName    string `json:"bank_name"`
}

// LLMLineItem represents a line item in the LLM response
type LLMLineItem struct {
	Number          int         `json:"number"`
	Code            string      `json:"code"`
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	Unit            string      `json:"unit"`
	Quantity        json.Number `json:"quantity"`
	UnitPrice       json.Number `json:"unit_price"`
	DiscountPercent json.Number `json:"discount_percent"`
	DiscountAmount  json.Number `json:"discount_amount"`
	Amount          json.Number `json:"amount"`
	VATRate         json.Number `json:"vat_rate"`
	VATAmount       json.Number `json:"vat_amount"`
	Total           json.Number `json:"total"`
}

func (e *Extractor) parseResponse(response string) (*model.Invoice, error) {
	// Extract JSON from response
	jsonStr := ExtractJSON(response)

	var llmResp LLMResponse
	if err := json.Unmarshal([]byte(jsonStr), &llmResp); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return e.convertToInvoice(&llmResp)
}

func (e *Extractor) convertToInvoice(resp *LLMResponse) (*model.Invoice, error) {
	// Determine document number (invoice_number takes precedence over receipt_number)
	docNumber := resp.InvoiceNumber
	if docNumber == "" {
		docNumber = resp.ReceiptNumber
	}

	inv := &model.Invoice{
		Number:         docNumber,
		Series:         resp.Series,
		Currency:       resp.Currency,
		Remarks:        resp.Notes,
		Provider:       model.ProviderUnknown, // LLM doesn't identify provider
		DocumentType:   parseDocumentType(resp.DocumentType),
		Cashier:        resp.Cashier,
		TerminalID:     resp.TerminalID,
		PaymentMethod:  resp.PaymentMethod,
		ReceiptNumber:  resp.ReceiptNumber,
		ReceiptTime:    resp.Time,
		AmountTendered: parseDecimal(resp.AmountTendered),
		Change:         parseDecimal(resp.Change),
	}

	// Parse date
	if resp.Date != "" {
		if t, err := parseDate(resp.Date); err == nil {
			inv.Date = t
		}
	}

	// Parse type
	inv.Type = parseInvoiceType(resp.Type)

	// Convert seller
	inv.Seller = model.Party{
		Name:        resp.Seller.Name,
		TaxID:       resp.Seller.TaxID,
		Address:     resp.Seller.Address,
		Phone:       resp.Seller.Phone,
		Email:       resp.Seller.Email,
		BankAccount: resp.Seller.BankAccount,
		BankName:    resp.Seller.BankName,
	}

	// Convert buyer
	inv.Buyer = model.Party{
		Name:        resp.Buyer.Name,
		TaxID:       resp.Buyer.TaxID,
		Address:     resp.Buyer.Address,
		Phone:       resp.Buyer.Phone,
		Email:       resp.Buyer.Email,
	}

	// Convert line items
	for _, item := range resp.Items {
		lineItem := model.LineItem{
			Number:      item.Number,
			Code:        item.Code,
			Name:        item.Name,
			Description: item.Description,
			Unit:        item.Unit,
		}

		// Parse decimals
		lineItem.Quantity = parseDecimal(item.Quantity)
		lineItem.UnitPrice = parseDecimal(item.UnitPrice)
		lineItem.Discount = parseDecimal(item.DiscountPercent)
		lineItem.DiscountAmt = parseDecimal(item.DiscountAmount)
		lineItem.Amount = parseDecimal(item.Amount)
		lineItem.VATAmount = parseDecimal(item.VATAmount)
		lineItem.Total = parseDecimal(item.Total)

		// Parse VAT rate
		if rate := parseDecimal(item.VATRate); !rate.IsZero() {
			lineItem.VATRate = model.VATRate(rate.IntPart())
		}

		inv.Items = append(inv.Items, lineItem)
	}

	// Parse totals
	inv.SubtotalAmount = parseDecimal(resp.Subtotal)
	inv.TaxAmount = parseDecimal(resp.TotalVAT)
	inv.TotalAmount = parseDecimal(resp.TotalAmount)

	// Set default currency if not provided
	if inv.Currency == "" {
		inv.Currency = "VND"
	}

	return inv, nil
}

func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)

	formats := []string{
		"2006-01-02",
		"02/01/2006",
		"2/1/2006",
		"02-01-2006",
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("cannot parse date: %s", s)
}

func parseInvoiceType(s string) model.InvoiceType {
	switch strings.ToLower(s) {
	case "replacement":
		return model.InvoiceTypeReplacement
	case "adjustment":
		return model.InvoiceTypeAdjustment
	default:
		return model.InvoiceTypeNormal
	}
}

func parseDocumentType(s string) model.DocumentType {
	switch strings.ToLower(s) {
	case "receipt":
		return model.DocumentTypeReceipt
	default:
		return model.DocumentTypeInvoice
	}
}

func parseDecimal(n json.Number) decimal.Decimal {
	if n == "" {
		return decimal.Zero
	}

	s := string(n)

	// Handle Vietnamese number format
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")

	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}

	return d
}
