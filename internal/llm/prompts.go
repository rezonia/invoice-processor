package llm

// Invoice extraction prompts

const SystemPromptInvoiceExtractor = `You are an expert invoice data extractor specializing in Vietnamese e-invoices (hóa đơn điện tử).

Your task is to extract structured data from invoice text or images. The invoices may be in Vietnamese or English.

Common Vietnamese invoice terms:
- Hóa đơn = Invoice
- Số hóa đơn = Invoice number
- Ký hiệu = Series/Symbol
- Ngày = Date
- Mã số thuế (MST) = Tax ID
- Người bán/Bên bán = Seller
- Người mua/Bên mua = Buyer
- Địa chỉ = Address
- Tên hàng hóa/dịch vụ = Product/Service name
- Đơn vị tính = Unit
- Số lượng = Quantity
- Đơn giá = Unit price
- Thành tiền = Amount
- Thuế suất = Tax rate
- Tiền thuế = Tax amount
- Tổng cộng = Total
- Cộng tiền hàng = Subtotal
- Thuế GTGT = VAT

Extract ALL information you can find. If a field is not present, omit it from the output.
Always output valid JSON that matches the specified schema.
Numbers should be parsed as integers (for VND) or decimals.
Dates should be in ISO 8601 format (YYYY-MM-DD).`

const UserPromptTextExtraction = `Extract invoice data from the following text:

---
%s
---

Output JSON with this structure:
{
  "invoice_number": "string",
  "series": "string",
  "date": "YYYY-MM-DD",
  "type": "normal|replacement|adjustment",
  "seller": {
    "name": "string",
    "tax_id": "string",
    "address": "string",
    "phone": "string",
    "email": "string",
    "bank_account": "string",
    "bank_name": "string"
  },
  "buyer": {
    "name": "string",
    "tax_id": "string",
    "address": "string",
    "phone": "string",
    "email": "string"
  },
  "items": [
    {
      "number": 1,
      "code": "string",
      "name": "string",
      "unit": "string",
      "quantity": 1,
      "unit_price": 100000,
      "discount_percent": 0,
      "discount_amount": 0,
      "amount": 100000,
      "vat_rate": 10,
      "vat_amount": 10000,
      "total": 110000
    }
  ],
  "subtotal": 100000,
  "total_discount": 0,
  "total_vat": 10000,
  "total_amount": 110000,
  "currency": "VND",
  "payment_method": "string",
  "notes": "string"
}`

const UserPromptImageExtraction = `Extract invoice data from this invoice image.

Output JSON with this structure:
{
  "invoice_number": "string",
  "series": "string",
  "date": "YYYY-MM-DD",
  "type": "normal|replacement|adjustment",
  "seller": {
    "name": "string",
    "tax_id": "string",
    "address": "string",
    "phone": "string",
    "email": "string",
    "bank_account": "string",
    "bank_name": "string"
  },
  "buyer": {
    "name": "string",
    "tax_id": "string",
    "address": "string",
    "phone": "string",
    "email": "string"
  },
  "items": [
    {
      "number": 1,
      "code": "string",
      "name": "string",
      "unit": "string",
      "quantity": 1,
      "unit_price": 100000,
      "discount_percent": 0,
      "discount_amount": 0,
      "amount": 100000,
      "vat_rate": 10,
      "vat_amount": 10000,
      "total": 110000
    }
  ],
  "subtotal": 100000,
  "total_discount": 0,
  "total_vat": 10000,
  "total_amount": 110000,
  "currency": "VND",
  "payment_method": "string",
  "notes": "string"
}

Extract all visible information from the invoice image. For any text that appears blurry or unclear, make your best attempt to read it.`

const UserPromptOCRCorrection = `The following is OCR-extracted text from a Vietnamese invoice. It may contain errors.

OCR Text:
---
%s
---

Please:
1. Correct any obvious OCR errors (especially in Vietnamese diacritics)
2. Extract the structured invoice data

Output JSON with the same structure as before.`

// Receipt extraction prompts

const SystemPromptReceiptExtractor = `You are an expert receipt data extractor specializing in retail POS receipts.

Your task is to extract structured data from receipt images. Receipts are typically thermal paper prints from stores, supermarkets, restaurants, or cafes.

Common receipt terms (Vietnamese/English):
- Hóa đơn bán hàng = Sales receipt
- Phiếu thanh toán = Payment slip
- Số HD/Receipt No = Receipt number
- Ngày/Date = Date
- Giờ/Time = Time
- Nhân viên/Cashier = Cashier
- Máy/Terminal = Terminal ID
- Tên hàng = Item name
- SL/Qty = Quantity
- Đơn giá = Unit price
- Thành tiền = Amount
- Tổng cộng/Total = Total
- Tiền mặt/Cash = Cash payment
- Thẻ/Card = Card payment
- Chuyển khoản = Bank transfer
- Ví điện tử/E-wallet = E-wallet (MoMo, ZaloPay, VNPay)
- Tiền khách đưa = Amount tendered
- Tiền thừa/Change = Change

Key differences from formal invoices:
- No buyer tax ID (customer info usually absent)
- No digital signature
- Often no VAT breakdown (included in price)
- Simpler format, shorter width

Extract ALL information you can find. If a field is not present, omit it.
Always set document_type to "receipt".
Output valid JSON matching the specified schema.`

const UserPromptReceiptExtraction = `Extract receipt data from this receipt image.

Output JSON with this structure:
{
  "document_type": "receipt",
  "receipt_number": "string",
  "date": "YYYY-MM-DD",
  "time": "HH:MM",
  "seller": {
    "name": "string (store name)",
    "address": "string",
    "phone": "string"
  },
  "cashier": "string",
  "terminal_id": "string",
  "items": [
    {
      "number": 1,
      "name": "string",
      "unit": "string",
      "quantity": 1,
      "unit_price": 50000,
      "amount": 50000
    }
  ],
  "subtotal": 100000,
  "total_vat": 0,
  "total_amount": 100000,
  "payment_method": "cash|card|e-wallet|transfer",
  "amount_tendered": 200000,
  "change": 100000,
  "currency": "VND"
}

Notes:
- For receipts, buyer info is typically absent - omit the buyer field
- If VAT is not shown separately, set total_vat to 0
- payment_method should be one of: cash, card, e-wallet, transfer
- amount_tendered and change are for cash payments only
- time field is optional, include if visible
- Extract store name from header/logo area`

const UserPromptAutoDetectExtraction = `Analyze this document image and extract data.

First, determine the document type:
- "invoice" = Formal tax invoice with seller/buyer tax IDs, VAT breakdown, invoice series/number
- "receipt" = Retail POS receipt, thermal paper, no buyer tax ID, simpler format

Then extract all available data.

Output JSON with this structure:
{
  "document_type": "invoice|receipt",
  "invoice_number": "string (for invoices)",
  "receipt_number": "string (for receipts)",
  "series": "string (for invoices only)",
  "date": "YYYY-MM-DD",
  "seller": {
    "name": "string",
    "tax_id": "string (for invoices)",
    "address": "string",
    "phone": "string"
  },
  "buyer": {
    "name": "string (for invoices)",
    "tax_id": "string (for invoices)"
  },
  "cashier": "string (for receipts)",
  "terminal_id": "string (for receipts)",
  "items": [...],
  "subtotal": 0,
  "total_vat": 0,
  "total_amount": 0,
  "payment_method": "string",
  "currency": "VND"
}

Include only fields that are present in the document.`
