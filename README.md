# ch.at - Universal Basic Chat

Minimalist LLM chat accessible through HTTP, SSH, DNS, and API. One binary, no JavaScript, no tracking.

## Usage

```bash
# Web (no JavaScript)
open https://ch.at

# Terminal
curl ch.at/?q=hello
ssh ch.at

# DNS tunneling
dig what-is-2+2.ch.at TXT

# API (OpenAI-compatible)
curl ch.at:8080/v1/chat/completions
```

## Design

- ~1,100 lines of Go, one external dependency
- Single static binary
- No accounts, no logs, no tracking
- Suckless-style configuration (edit source)

## Privacy

We take a "can't be evil" approach:

- No authentication or user tracking
- No server-side conversation storage
- No logs whatsoever
- Web history stored client-side only

**⚠️ PRIVACY WARNING**: Your queries are sent to LLM providers (OpenAI, Anthropic, etc.) who may log and store them according to their policies. While ch.at doesn't log anything, the upstream providers might. Never send passwords, API keys, or sensitive information.

**Current Production Model**: OpenAI's GPT-4o. We plan to expand model access in the future.

## Installation

Create `llm.go` (gitignored):

```go
// llm.go - Create this file (it's gitignored)
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func getLLMResponse(ctx context.Context, prompt string) (string, error) {
	var response strings.Builder
	stream, err := getLLMResponseStream(ctx, prompt)
	if err != nil {
		return "", err
	}
	for chunk := range stream {
		response.WriteString(chunk)
	}
	return response.String(), nil
}

func getLLMResponseStream(ctx context.Context, prompt string) (<-chan string, error) {
	endpoint := "https://api.openai.com/v1/chat/completions"
	key := "YOUR-OPENAI-API-KEY-HERE"  // Replace with your key
	
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": true,
	}
	
	// ... rest of implementation
}
```

Then build:
```bash
go build -o chat .
sudo ./chat  # Needs root for ports 80/443/53/22

# Optional: build selftest tool
go build -o selftest ./cmd/selftest
```

To run on high ports, edit the constants in `chat.go` and rebuild:
```go
const (
    HTTP_PORT   = 8080  // Instead of 80
    HTTPS_PORT  = 0     // Disabled
    SSH_PORT    = 2222  // Instead of 22
    DNS_PORT    = 0     // Disabled
    OPENAI_PORT = 8080  // Same as HTTP
)
```

Then:
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
- Remove protocol: Delete its .go file

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

