# ch.at - Universal Basic Chat

A lightweight language model chat service accessible through HTTP, SSH, DNS, and API. One binary, no JavaScript, no tracking.

## Usage

```bash
# Web (no JavaScript)
open https://ch.at

# Terminal
curl ch.at/?q=hello             # Query parameter (handles special chars)
curl ch.at/what-is-rust         # Path-based (cleaner URLs, hyphens become spaces)
ssh ch.at

# DNS tunneling
dig what-is-2+2.ch.at TXT

# API (OpenAI-compatible)
curl ch.at/v1/chat/completions
```

## Design

- ~1,200 lines of Go, three dependencies
- Single static binary
- No accounts, no logs, no tracking
- Configuration through source code (edit and recompile)


## Privacy

Privacy by design:

- No authentication or user tracking
- No server-side conversation storage
- No logs whatsoever
- Web history stored client-side only

**⚠️ PRIVACY WARNING**: Your queries are sent to LLM providers (OpenAI, Anthropic, etc.) who may log and store them according to their policies. While ch.at doesn't log anything, the upstream providers might. Never send passwords, API keys, or sensitive information.

**Current Production Model**: OpenAI's GPT-4o. We plan to expand model access in the future.

## Installation

### Quick Start

```bash
# Copy the example LLM configuration (llm.go is gitignored)
cp llm.go.example llm.go

# Edit llm.go and add your API key
# Supports OpenAI, Anthropic Claude, or local models (Ollama)

# For HTTPS, you'll need cert.pem and key.pem files:
# Option 1: Use Let's Encrypt (recommended for production)
# Option 2: Use your existing certificates
# Option 3: Self-signed for testing:
#   openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes

# Build and run
go build -o chat .
sudo ./chat  # Needs root for ports 80/443/53/22
```

### Testing

```bash
# Build the self-test tool
go build -o selftest ./cmd/selftest

# Run all protocol tests
./selftest http://localhost

# Test specific queries
curl localhost/what-is-go
curl localhost/?q=hello
```

### High Port Configuration

To run without sudo, edit the constants in `chat.go`:

```go
const (
    HTTP_PORT   = 8080  // Instead of 80
    HTTPS_PORT  = 0     // Disabled
    SSH_PORT    = 2222  // Instead of 22
    DNS_PORT    = 0     // Disabled
)
```

Then build:
```bash
go build -o chat .
./chat  # No sudo needed for high ports

# Test the service
./selftest http://localhost:8080
```

## Configuration

Edit constants in source files:
- Ports: `chat.go` (set to 0 to disable)
- Rate limits: `util.go`
- Remove service: Delete its .go file

## Limitations

- **DNS**: Responses limited to ~500 chars due to protocol constraints
- **History**: Limited to 2KB in web interface to prevent URL overflow
- **Rate limiting**: Basic IP-based limiting to prevent abuse
- **No encryption**: SSH is encrypted, but HTTP/DNS are not

## License

MIT License - see LICENSE file

## Contributing

Before adding features:
- Does it increase accessibility?
- Is it under 50 lines?
- Is it necessary?

