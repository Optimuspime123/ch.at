package main

import (
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
    <p><a href="/">Clear History</a> • <a href="https://github.com/Deep-ai-inc/ch.at#readme">About</a></p>
</body>
</html>`

func StartHTTPServer(port int) error {
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/v1/chat/completions", handleChatCompletions)

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("HTTP server listening on %s\n", addr)
	return http.ListenAndServe(addr, nil)
}

func StartHTTPSServer(port int, certFile, keyFile string) error {
	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("HTTPS server listening on %s\n", addr)
	return http.ListenAndServeTLS(addr, certFile, keyFile, nil)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if !rateLimitAllow(r.RemoteAddr) {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
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
		query = r.URL.Query().Get("q")
		// Also support path-based queries like /what-is-go
		if query == "" && r.URL.Path != "/" {
			query = strings.ReplaceAll(strings.TrimPrefix(r.URL.Path, "/"), "-", " ")
		}
	}


	if query != "" {
		// Build prompt with history
		prompt = query
		if history != "" {
			prompt = history + "Q: " + query
		}

		response, err := LLM(prompt, nil)
		if err != nil {
			content = err.Error()
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
				// Keep roughly last 600 characters (UTF-8 safe)
				runes := []rune(content)
				if len(runes) > 600 {
					content = string(runes[len(runes)-600:])
				}
			}
		}
	} else if history != "" {
		content = history
	}

	accept := r.Header.Get("Accept")
	wantsJSON := strings.Contains(accept, "application/json")
	wantsHTML := strings.Contains(accept, "text/html")
	wantsStream := strings.Contains(accept, "text/event-stream")

	// Stream for curl when requested
	if wantsStream && query != "" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Stream response
		ch := make(chan string)
		go func() {
			if _, err := LLM(prompt, ch); err != nil {
				// Send error as SSE event
				fmt.Fprintf(w, "data: Error: %s\n\n", err.Error())
				flusher.Flush()
			}
		}()

		for chunk := range ch {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		return
	}

	// Return JSON for API requests, HTML for browsers, plain text for curl
	if wantsJSON && jsonResponse != "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprint(w, jsonResponse)
	} else if wantsHTML {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, minimalHTML, html.EscapeString(content), html.EscapeString(content))
	} else {
		// Default to plain text for curl and other tools
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, content)
	}
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Index   int     `json:"index"`
	Message Message `json:"message"`
}

func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if !rateLimitAllow(r.RemoteAddr) {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Convert messages to format for LLM
	messages := make([]map[string]string, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}
	
	
	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}
		
		// Stream response
		ch := make(chan string)
		go LLM(messages, ch)
		
		for chunk := range ch {
			resp := map[string]interface{}{
				"id": fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
				"object": "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model": req.Model,
				"choices": []map[string]interface{}{{
					"index": 0,
					"delta": map[string]string{"content": chunk},
				}},
			}
			data, err := json.Marshal(resp)
			if err != nil {
				fmt.Fprintf(w, "data: Failed to marshal response\n\n")
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		
	} else {
		response, err := LLM(messages, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		chatResp := ChatResponse{
			ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []Choice{{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: response,
				},
			}},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResp)
	}
}



