# Invoice Processor

A Go library for processing Vietnam e-invoices with hybrid extraction support (XML, PDF, and image-based invoices).

## Features

- **Multi-Format Support**: Process XML, PDF, and image invoices
- **Multiple Providers**: Support for TCT, VNPT, MISA, Viettel, and FPT invoice formats
- **Hybrid Extraction Pipeline**: Template matching → OCR + LLM Text → Pure LLM Vision
- **Digital Signature Verification**: Verify XMLDSig and PDF signatures with Vietnam CA trust store
- **Cost-Optimized**: Falls back to more expensive methods only when needed
- **CLI Tool**: Command-line interface for processing invoices
- **REST API**: Gin-based HTTP server for microservice deployment
- **gRPC Support**: Protocol buffer definitions for high-performance services
- **Financial Precision**: Uses `shopspring/decimal` for accurate calculations

## Installation

```bash
go get github.com/rezonia/invoice-processor
```

## Quick Start

### As a Library

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/rezonia/invoice-processor/pkg/invoicelib"
)

func main() {
    // Create processor with options
    opts := invoicelib.DefaultPipelineOptions()
    opts.LLMAPIKey = os.Getenv("LLM_API_KEY")
    proc := invoicelib.NewProcessor(opts)

    // Process an invoice file
    file, _ := os.Open("invoice.xml")
    defer file.Close()

    result, err := proc.Process(context.Background(), file)
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Invoice #%s: %s VND", result.Invoice.Number, result.Invoice.TotalAmount)
    log.Printf("Extraction method: %s (confidence: %.2f)", result.Method, result.Confidence)
}
```

### CLI Usage

```bash
# Build the CLI
go build -o invoice-processor ./cmd/invoice-processor

# Process XML invoice
./invoice-processor process invoice.xml

# Process multiple files with JSON output
./invoice-processor process *.xml --format json --output results.json

# Validate invoice
./invoice-processor validate invoice.xml

# Get file info
./invoice-processor info invoice.pdf

# Start HTTP API server
./invoice-processor serve --address :8080 --api-key $LLM_API_KEY

# Verify digital signature
./invoice-processor verify invoice.xml

# Verify with custom CA certificate
./invoice-processor verify --ca-file custom-ca.crt invoice.xml

# Skip OCSP revocation check
./invoice-processor verify --skip-ocsp invoice.pdf
```

### REST API

```bash
# Start the server
./invoice-processor serve --address :8080

# Health check
curl http://localhost:8080/health

# Process XML invoice
curl -X POST http://localhost:8080/api/v1/process/xml \
  -H "Content-Type: application/xml" \
  --data-binary @invoice.xml

# Process with auto-detection
curl -X POST http://localhost:8080/api/v1/process/auto \
  --data-binary @invoice.pdf

# Validate invoice
curl -X POST http://localhost:8080/api/v1/validate \
  --data-binary @invoice.xml

# Get file info
curl -X POST http://localhost:8080/api/v1/info \
  --data-binary @invoice.pdf

# Verify digital signature
curl -X POST http://localhost:8080/api/v1/verify \
  -H "Content-Type: application/xml" \
  --data-binary @invoice.xml
```

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/process/xml` | Process XML invoice |
| POST | `/api/v1/process/pdf` | Process PDF invoice |
| POST | `/api/v1/process/image` | Process image invoice |
| POST | `/api/v1/process/auto` | Auto-detect format and process |
| POST | `/api/v1/validate` | Validate invoice (XML only) |
| POST | `/api/v1/info` | Get file format information |
| POST | `/api/v1/verify` | Verify digital signature |

## Supported Invoice Providers

| Provider | XML Format | PDF Template | Status |
|----------|------------|--------------|--------|
| TCT (Tax Authority) | Yes | Yes | Full support |
| VNPT | Yes | Yes | Full support |
| MISA | Yes | Yes | Full support |
| Viettel | Yes | Yes | Full support |
| FPT | Yes | Yes | Full support |

## Extraction Pipeline

The processor uses a cost-optimized hybrid extraction approach:

1. **XML Parsing** (Confidence: 1.0)
   - Direct parsing for XML invoices
   - Provider-specific adapters for different formats

2. **PDF Template Extraction** (Confidence: 0.95)
   - Pattern matching using provider-specific templates
   - Regex-based field extraction from PDF text

3. **LLM Text Extraction** (Confidence: varies)
   - OCR text fed to LLM for structured extraction
   - Used when template matching fails

4. **LLM Vision Extraction** (Confidence: varies)
   - Direct image analysis for scanned/image invoices
   - Fallback for complex PDF layouts

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LLM_API_KEY` | API key for LLM provider | - |
| `LLM_BASE_URL` | LLM API base URL | `https://openrouter.ai/api/v1` |
| `LLM_MODEL` | Model for text extraction | `anthropic/claude-3.5-sonnet` |
| `LLM_VISION_MODEL` | Model for vision/image extraction | `anthropic/claude-3.5-sonnet` |

### LLM Provider Configuration

The processor uses OpenAI-compatible APIs. You can use OpenRouter (default) or connect directly to other providers.

#### OpenRouter (Default)

```bash
export LLM_API_KEY=sk-or-v1-xxx
# No need to set LLM_BASE_URL, defaults to OpenRouter
```

#### OpenAI Direct

```bash
export LLM_API_KEY=sk-xxx
export LLM_BASE_URL=https://api.openai.com/v1
export LLM_MODEL=gpt-4o
export LLM_VISION_MODEL=gpt-4o
```

#### Anthropic (via OpenAI-compatible proxy)

```bash
export LLM_API_KEY=sk-ant-xxx
export LLM_BASE_URL=https://your-anthropic-proxy.com/v1
```

#### Local LLM (Ollama, LM Studio, etc.)

```bash
export LLM_API_KEY=not-needed
export LLM_BASE_URL=http://localhost:11434/v1
export LLM_MODEL=llava
export LLM_VISION_MODEL=llava
```

### Recommended Models

| Provider | Model ID | Vision | Notes |
|----------|----------|--------|-------|
| Anthropic | `anthropic/claude-3.5-sonnet` | ✅ | Best accuracy (default) |
| Anthropic | `anthropic/claude-3-haiku` | ✅ | Faster, lower cost |
| OpenAI | `openai/gpt-4o` | ✅ | High accuracy |
| OpenAI | `openai/gpt-4o-mini` | ✅ | Good balance of cost/quality |
| Google | `google/gemini-flash-1.5` | ✅ | Fast, good for high volume |

### Pipeline Options

```go
opts := invoicelib.PipelineOptions{
    TemplateThreshold: 0.90,  // Minimum confidence for template
    LLMThreshold:      0.85,  // Minimum confidence for LLM
    ReviewThreshold:   0.70,  // Below this, flag for review
    LLMAPIKey:         "...", // OpenRouter API key (env: LLM_API_KEY)
    LLMBaseURL:        "https://openrouter.ai/api/v1", // env: LLM_BASE_URL
    LLMModel:          "anthropic/claude-3.5-sonnet",  // env: LLM_MODEL
    LLMVisionModel:    "anthropic/claude-3.5-sonnet",  // env: LLM_VISION_MODEL
    EnableLLM:         true,
    EnableOCR:         true,
    ValidateAfterExtraction: true,
}
```

## Project Structure

```
.
├── api/
│   └── grpc/
│       └── proto/           # Protocol buffer definitions
├── cmd/
│   └── invoice-processor/   # CLI application
│       └── cmd/             # Cobra commands
├── internal/
│   ├── decimal/             # Financial decimal helpers
│   ├── llm/                 # OpenRouter LLM integration
│   ├── model/               # Core data types
│   ├── parser/
│   │   ├── pdf/             # PDF extraction
│   │   └── xml/             # XML adapters
│   ├── processor/           # Hybrid extraction pipeline
│   ├── server/              # Gin HTTP server
│   └── signature/           # Digital signature verification
│       ├── pdf/             # PDF signature (pdfsig wrapper)
│       ├── trust/           # Vietnam CA trust store
│       └── xml/             # XMLDSig verification
└── pkg/
    └── invoicelib/          # Public API
```

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run with coverage
go test ./... -cover

# Run benchmarks
go test ./... -bench=.
```

## Docker

### Build

```bash
docker build -t invoice-processor .
```

### Run Server

```bash
# Basic usage
docker run -p 8080:8080 -e LLM_API_KEY=your-key invoice-processor

# With custom LLM configuration
docker run -p 8080:8080 \
  -e LLM_API_KEY=your-key \
  -e LLM_MODEL=anthropic/claude-3.5-sonnet \
  -e LLM_VISION_MODEL=anthropic/claude-3.5-sonnet \
  invoice-processor
```

### Process Files (CLI mode)

```bash
# Process a local file
docker run --rm -v $(pwd):/data invoice-processor process /data/invoice.xml

# Process with output
docker run --rm -v $(pwd):/data invoice-processor process /data/invoice.pdf \
  -e LLM_API_KEY=your-key \
  -o /data/result.json
```

### Docker Compose

```yaml
version: '3.8'
services:
  invoice-processor:
    build: .
    ports:
      - "8080:8080"
    environment:
      - LLM_API_KEY=${LLM_API_KEY}
      - LLM_MODEL=anthropic/claude-3.5-sonnet
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8080/health"]
      interval: 30s
      timeout: 3s
      retries: 3
```

## Development

### Prerequisites

- Go 1.23+
- Docker (optional, for containerized deployment)
- (Optional) protoc for gRPC code generation
- (Optional) pdfsig for PDF signature verification

#### Installing pdfsig

PDF signature verification requires `pdfsig` from poppler-utils:

```bash
# macOS
brew install poppler

# Ubuntu/Debian
sudo apt-get install poppler-utils

# Alpine Linux (Docker)
apk add poppler-utils

# Fedora/RHEL
sudo dnf install poppler-utils
```

> **Note:** XML signature verification works without any external dependencies (pure Go).
> PDF signature verification gracefully falls back to "unavailable" if pdfsig is not installed.

### Build

```bash
# Build library
go build ./...

# Build CLI
go build -o invoice-processor ./cmd/invoice-processor

# Run tests
go test ./...
```

### Generate gRPC Code

```bash
protoc --go_out=. --go-grpc_out=. api/grpc/proto/invoice.proto
```

## License

MIT License
