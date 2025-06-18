package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

const minimalHTML = `<!DOCTYPE html>
<html>
<head>
    <title>ch.at</title>
    <style>
        body { text-align: center; margin: 40px; }
        pre { text-align: left; max-width: 600px; margin: 20px auto; padding: 20px; 
              white-space: pre-wrap; word-wrap: break-word; }
        input[type="text"] { width: 300px; }
    </style>
</head>
<body>
    <h1>ch.at</h1>
    <p><i>pronounced "ch-dot-at"</i></p>
    <pre>%s</pre>
    <form method="POST" action="/">
        <input type="text" name="q" placeholder="Type your message..." autofocus>
        <textarea name="h" style="display:none">%s</textarea>
        <input type="submit" value="Send">
    </form>
    <p><a href="/">Clear History</a> • <a href="https://github.com/ch-at/ch.at#readme">About</a></p>
</body>
</html>`

type HTTPServer struct {
	port int
}

func NewHTTPServer(port int) *HTTPServer {
	return &HTTPServer{port: port}
}

func (s *HTTPServer) Start() error {
	http.HandleFunc("/", s.handleRoot)

	addr := fmt.Sprintf(":%d", s.port)
	fmt.Printf("HTTP server listening on %s\n", addr)
	return http.ListenAndServe(addr, nil)
}

func (s *HTTPServer) StartTLS(port int, certFile, keyFile string) error {
	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("HTTPS server listening on %s\n", addr)
	return http.ListenAndServeTLS(addr, certFile, keyFile, nil)
}

func (s *HTTPServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	if !rateLimiter.Allow(r.RemoteAddr) {
		http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
		return
	}

	var query, history, prompt string
	content := ""
	jsonResponse := ""

	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		query = r.FormValue("q")
		history = r.FormValue("h")

		// Limit history size to prevent abuse
		if len(history) > 2048 {
			history = history[len(history)-2048:]
		}

		// If no form fields, treat body as raw query (for curl)
		if query == "" {
			body, err := io.ReadAll(io.LimitReader(r.Body, 4096)) // Limit body size
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			query = string(body)
		}
	} else {
		// GET request - no history
		query = r.URL.Query().Get("q")
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if query != "" {
		// Build prompt with history
		prompt = query
		if history != "" {
			prompt = history + "Q: " + query
		}

		response, err := getLLMResponse(ctx, prompt)
		if err != nil {
			content = fmt.Sprintf("Error: %s", err.Error())
			errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
			jsonResponse = string(errJSON)
		} else {
			// Store JSON response
			respJSON, _ := json.Marshal(map[string]string{
				"question": query,
				"answer":   response,
			})
			jsonResponse = string(respJSON)

			// Append to history
			newExchange := fmt.Sprintf("Q: %s\nA: %s\n\n", query, response)
			if history != "" {
				content = history + newExchange
			} else {
				content = newExchange
			}
			// Trim history if too long (UTF-8 safe)
			if len(content) > 2048 {
				// UTF-8 continuation bytes start with 10xxxxxx (0x80-0xBF)
				// Find a character boundary to avoid splitting multi-byte chars
				for i := len(content) - 2048; i < len(content)-2040; i++ {
					if content[i]&0xC0 != 0x80 { // Not a continuation byte
						content = content[i:]
						break
					}
				}
			}
		}
	} else if history != "" {
		content = history
	}

	accept := r.Header.Get("Accept")

	// Stream for curl when requested
	if strings.Contains(accept, "text/event-stream") && query != "" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		stream, err := getLLMResponseStream(ctx, prompt)
		if err != nil {
			fmt.Fprintf(w, "data: Error: %s\n\n", err.Error())
			return
		}

		for chunk := range stream {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		return
	}

	// Return JSON for API requests, HTML for browsers, plain text for curl
	if strings.Contains(accept, "application/json") && jsonResponse != "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprint(w, jsonResponse)
	} else if strings.Contains(accept, "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, minimalHTML, html.EscapeString(content), html.EscapeString(content))
	} else {
		// Default to plain text for curl and other tools
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, content)
	}
}
