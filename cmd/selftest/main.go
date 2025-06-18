package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: selftest <base-url>")
		fmt.Println("Example: selftest http://localhost:8080")
		os.Exit(1)
	}

	baseURL := strings.TrimSuffix(os.Args[1], "/")
	passed := 0
	failed := 0

	// Test 1: Basic HTTP GET
	fmt.Print("Testing HTTP GET... ")
	resp, err := http.Get(baseURL + "/?q=hello")
	if err == nil && resp.StatusCode == 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(body), "hello") || strings.Contains(string(body), "Hello") {
			fmt.Println("✓")
			passed++
		} else {
			fmt.Println("✗ (unexpected response)")
			failed++
		}
	} else {
		fmt.Println("✗ (request failed)")
		failed++
	}

	// Test 2: HTTP POST
	fmt.Print("Testing HTTP POST... ")
	resp, err = http.Post(baseURL+"/", "text/plain", strings.NewReader("What is 2+2?"))
	if err == nil && resp.StatusCode == 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(body), "4") || strings.Contains(string(body), "four") {
			fmt.Println("✓")
			passed++
		} else {
			fmt.Println("✗ (unexpected response)")
			failed++
		}
	} else {
		fmt.Println("✗ (request failed)")
		failed++
	}

	// Test 3: JSON API
	fmt.Print("Testing JSON API... ")
	req, _ := http.NewRequest("GET", baseURL+"/?q=test", nil)
	req.Header.Set("Accept", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err == nil && resp.StatusCode == 200 {
		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if result["question"] == "test" && result["answer"] != "" {
			fmt.Println("✓")
			passed++
		} else {
			fmt.Println("✗ (invalid JSON response)")
			failed++
		}
	} else {
		fmt.Println("✗ (request failed)")
		failed++
	}

	// Test 4: OpenAI API compatibility
	fmt.Print("Testing OpenAI API... ")
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": "Say 'test passed'"},
		},
	}
	jsonData, _ := json.Marshal(payload)
	// OpenAI API is on port 8080 in production
	apiURL := "http://localhost:8080/v1/chat/completions"
	resp, err = http.Post(apiURL, "application/json", bytes.NewReader(jsonData))
	if err == nil && resp.StatusCode == 200 {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
			fmt.Println("✓")
			passed++
		} else {
			fmt.Println("✗ (invalid response format)")
			failed++
		}
	} else {
		fmt.Println("✗ (request failed)")
		failed++
	}

	// Test 5: Rate limiting (default is 100 requests/minute)
	fmt.Print("Testing rate limiting... ")
	rateLimitHit := false
	// Make requests quickly to trigger rate limit
	// Use empty query to avoid LLM calls
	for i := 0; i < 110; i++ {
		resp, err := http.Get(baseURL + "/")
		if err == nil {
			if resp.StatusCode == 429 {
				rateLimitHit = true
				resp.Body.Close()
				break
			}
			resp.Body.Close()
		}
	}
	if rateLimitHit {
		fmt.Println("✓")
		passed++
	} else {
		fmt.Println("✗ (rate limit not enforced)")
		failed++
	}

	// Summary
	fmt.Printf("\nTests passed: %d/%d\n", passed, passed+failed)
	if failed > 0 {
		os.Exit(1)
	}
}