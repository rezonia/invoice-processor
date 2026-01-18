package model

import (
	"time"

	"github.com/shopspring/decimal"
)

// Provider represents e-invoice provider
type Provider string

const (
	ProviderTCT     Provider = "TCT"
	ProviderVNPT    Provider = "VNPT"
	ProviderMISA    Provider = "MISA"
	ProviderViettel Provider = "VIETTEL"
	ProviderFPT     Provider = "FPT"
	ProviderUnknown Provider = "UNKNOWN"
)

// VATRate represents valid Vietnam VAT rates
type VATRate int

const (
	VATRate0  VATRate = 0
	VATRate5  VATRate = 5
	VATRate10 VATRate = 10
)

// InvoiceType represents invoice type
type InvoiceType string

const (
	InvoiceTypeNormal      InvoiceType = "Normal"
	InvoiceTypeReplacement InvoiceType = "Replacement"
	InvoiceTypeAdjustment  InvoiceType = "Adjustment"
)

// DocumentType distinguishes invoice from receipt
type DocumentType string

const (
	DocumentTypeInvoice DocumentType = "invoice"
	DocumentTypeReceipt DocumentType = "receipt"
)

// Invoice represents a Vietnam e-invoice
type Invoice struct {
	// Unique identifier
	ID string `json:"id"`

	// Header
	Number   string    `json:"number"`   // Invoice number (1-6 digits)
	Series   string    `json:"series"`   // Invoice series (2-5 chars)
	Date     time.Time `json:"date"`     // Invoice date
	Type     InvoiceType `json:"type"`   // Normal, Replacement, Adjustment
	Provider Provider  `json:"provider"` // TCT, VNPT, MISA, etc.

	// Parties
	Seller Party `json:"seller"`
	Buyer  Party `json:"buyer"`

	// Line Items
	Items []LineItem `json:"items"`

	// Totals (VND, no decimals in final amount)
	SubtotalAmount decimal.Decimal `json:"subtotal_amount"`
	TaxAmount      decimal.Decimal `json:"tax_amount"`
	TotalAmount    decimal.Decimal `json:"total_amount"`

	// Currency
	Currency     string          `json:"currency"` // "VND"
	ExchangeRate decimal.Decimal `json:"exchange_rate,omitempty"`

	// Optional
	Remarks      string `json:"remarks,omitempty"`
	PaymentTerms string `json:"payment_terms,omitempty"`

	// Document type and receipt-specific fields
	DocumentType   DocumentType    `json:"document_type"`
	Cashier        string          `json:"cashier,omitempty"`
	TerminalID     string          `json:"terminal_id,omitempty"`
	PaymentMethod  string          `json:"payment_method,omitempty"`
	ReceiptNumber  string          `json:"receipt_number,omitempty"`
	ReceiptTime    string          `json:"receipt_time,omitempty"`    // HH:MM format
	AmountTendered decimal.Decimal `json:"amount_tendered,omitempty"` // Cash given
	Change         decimal.Decimal `json:"change,omitempty"`          // Change returned

	// Signature (if signed)
	Signature *Signature `json:"signature,omitempty"`

	// Metadata
	RawXML     []byte `json:"-"`           // Original XML for audit
	SourceFile string `json:"source_file"` // Source file path
}

// Party represents seller or buyer
type Party struct {
	Name        string `json:"name"`
	TaxID       string `json:"tax_id"`  // 10 digits
	Address     string `json:"address"`
	Phone       string `json:"phone,omitempty"`
	Email       string `json:"email,omitempty"`
	BankAccount string `json:"bank_account,omitempty"`
	BankName    string `json:"bank_name,omitempty"`
}

// LineItem represents invoice line item
type LineItem struct {
	Number      int             `json:"number"`
	Code        string          `json:"code,omitempty"` // Optional item code
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Unit        string          `json:"unit"` // "piece", "kg", "meter"
	Quantity    decimal.Decimal `json:"quantity"`
	UnitPrice   decimal.Decimal `json:"unit_price"`
	Discount    decimal.Decimal `json:"discount,omitempty"` // Discount percentage
	VATRate     VATRate         `json:"vat_rate"`

	// Calculated
	Amount      decimal.Decimal `json:"amount"`       // Quantity * UnitPrice
	DiscountAmt decimal.Decimal `json:"discount_amt"` // Amount * Discount%
	VATAmount   decimal.Decimal `json:"vat_amount"`   // (Amount - Discount) * VATRate%
	Total       decimal.Decimal `json:"total"`        // Amount - Discount + VAT
}

// Signature represents digital signature data
type Signature struct {
	Value          string    `json:"value"` // Base64 encoded
	Date           time.Time `json:"date"`
	SignerName     string    `json:"signer_name"`
	SignerPosition string    `json:"signer_position,omitempty"`
	CertSerial     string    `json:"cert_serial,omitempty"`
}

// CalculateLineItem computes line item totals
func (li *LineItem) Calculate() {
	// Amount = Quantity * UnitPrice
	li.Amount = li.Quantity.Mul(li.UnitPrice)

	// DiscountAmt = Amount * (Discount / 100)
	if !li.Discount.IsZero() {
		li.DiscountAmt = li.Amount.Mul(li.Discount).Div(decimal.NewFromInt(100)).Round(0)
	}

	// VATAmount = (Amount - DiscountAmt) * (VATRate / 100)
	taxableAmount := li.Amount.Sub(li.DiscountAmt)
	li.VATAmount = taxableAmount.Mul(decimal.NewFromInt(int64(li.VATRate))).Div(decimal.NewFromInt(100)).Round(0)

	// Total = Amount - DiscountAmt + VATAmount
	li.Total = taxableAmount.Add(li.VATAmount).Round(0)
}

// CalculateTotals computes invoice totals from line items
func (inv *Invoice) CalculateTotals() {
	subtotal := decimal.Zero
	tax := decimal.Zero

	for i := range inv.Items {
		inv.Items[i].Calculate()
		subtotal = subtotal.Add(inv.Items[i].Amount.Sub(inv.Items[i].DiscountAmt))
		tax = tax.Add(inv.Items[i].VATAmount)
	}

	inv.SubtotalAmount = subtotal.Round(0)
	inv.TaxAmount = tax.Round(0)
	inv.TotalAmount = subtotal.Add(tax).Round(0)
}
