package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)


// extractLLMResponse extracts just the LLM response from various formats
func extractLLMResponse(body string, contentType string) string {
	body = strings.TrimSpace(body)
	
	// For error responses, return empty to fail the test
	if strings.Contains(body, "error") || strings.Contains(body, "Error") {
		return ""
	}
	
	// For JSON responses
	if strings.Contains(contentType, "json") {
		var result map[string]string
		if err := json.Unmarshal([]byte(body), &result); err == nil {
			if answer, ok := result["answer"]; ok {
				return strings.TrimSpace(answer)
			}
		}
		return ""
	}
	
	// For plain text Q&A format, extract just the answer
	if strings.Contains(body, "\nA: ") {
		lines := strings.Split(body, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "A: ") {
				return strings.TrimSpace(strings.TrimPrefix(line, "A: "))
			}
		}
	}
	
	// Otherwise return trimmed body
	return body
}

func checkResponse(resp *http.Response, err error, passed, failed *int) {
	if err != nil {
		fmt.Println("✗ (request failed)")
		*failed++
		return
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != 200 {
		fmt.Printf("✗ (status %d)\n", resp.StatusCode)
		*failed++
		return
	}
	
	contentType := resp.Header.Get("Content-Type")
	llmResponse := extractLLMResponse(string(body), contentType)
	
	// Check for exact match "pass"
	if llmResponse == "pass" {
		fmt.Println("✓")
		*passed++
	} else {
		// Show what we got instead
		preview := llmResponse
		if preview == "" {
			preview = "error or empty response"
		} else if len(preview) > 50 {
			preview = preview[:50] + "..."
		}
		fmt.Printf("✗ (expected 'pass', got: %q)\n", preview)
		*failed++
	}
}


func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: selftest <base-url>")
		fmt.Println("Example: selftest http://localhost:8080")
		os.Exit(1)
	}

	baseURL := strings.TrimSuffix(os.Args[1], "/")
	
	// Extract hostname from URL for SSH/DNS tests
	hostname := "localhost"
	if u, err := url.Parse(baseURL); err == nil && u.Hostname() != "" {
		hostname = u.Hostname()
	}
	
	passed := 0
	failed := 0

	// Add delay between tests to avoid rate limiting
	testDelay := 700 * time.Millisecond

	// Test 1: Basic HTTP GET
	fmt.Print("Testing HTTP GET... ")
	resp, err := http.Get(baseURL + "/?q=repeat+verbatim+the+word+pass")
	checkResponse(resp, err, &passed, &failed)

	time.Sleep(testDelay)

	// Test 2: HTTP POST
	fmt.Print("Testing HTTP POST... ")
	resp, err = http.Post(baseURL+"/", "text/plain", strings.NewReader("repeat verbatim the word pass"))
	checkResponse(resp, err, &passed, &failed)

	time.Sleep(testDelay)

	// Test 3: Path-based query
	fmt.Print("Testing path-based query... ")
	resp, err = http.Get(baseURL + "/repeat-verbatim-the-word-pass")
	checkResponse(resp, err, &passed, &failed)

	time.Sleep(testDelay)

	// Test 4: JSON API
	fmt.Print("Testing JSON API... ")
	req, _ := http.NewRequest("GET", baseURL+"/?q=repeat+verbatim+the+word+pass", nil)
	req.Header.Set("Accept", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err == nil && resp.StatusCode == 200 {
		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if result["question"] == "repeat verbatim the word pass" && result["answer"] == "pass" {
			fmt.Println("✓")
			passed++
		} else {
			answer := result["answer"]
			if answer == "" {
				answer = "no answer field"
			}
			fmt.Printf("✗ (expected 'pass', got: %q)\n", answer)
			failed++
		}
	} else {
		fmt.Println("✗ (request failed)")
		failed++
	}

	time.Sleep(testDelay)

	// Test 5: OpenAI API compatibility
	fmt.Print("Testing OpenAI API... ")
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": "repeat verbatim the word pass"},
		},
	}
	jsonData, _ := json.Marshal(payload)
	// OpenAI API is on main HTTP port when OPENAI_PORT=0
	apiURL := baseURL + "/v1/chat/completions"
	resp, err = http.Post(apiURL, "application/json", bytes.NewReader(jsonData))
	if err == nil && resp.StatusCode == 200 {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok {
						content = strings.TrimSpace(content)
						if content == "pass" {
							fmt.Println("✓")
							passed++
						} else {
							fmt.Printf("✗ (expected 'pass', got: %q)\n", content)
							failed++
						}
					} else {
						fmt.Println("✗ (no content in message)")
						failed++
					}
				} else {
					fmt.Println("✗ (invalid message format)")
					failed++
				}
			} else {
				fmt.Println("✗ (invalid choice format)")
				failed++
			}
		} else {
			fmt.Println("✗ (invalid response format)")
			failed++
		}
	} else {
		fmt.Println("✗ (request failed)")
		failed++
	}

	time.Sleep(testDelay)

	// Test 6: SSH protocol
	fmt.Print("Testing SSH protocol... ")
	config := &ssh.ClientConfig{
		User: "anonymous", 
		Auth: []ssh.AuthMethod{
			ssh.Password(""),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
		ClientVersion:   "SSH-2.0-Go", // Explicitly set version
	}
	
	// Use 127.0.0.1 instead of localhost to avoid IPv6 issues
	sshHost := hostname
	if hostname == "localhost" {
		sshHost = "127.0.0.1"
	}
	
	sshClient, err := ssh.Dial("tcp", sshHost+":22", config)
	if err == nil {
		defer sshClient.Close()
		
		// Create a session and send a real query
		session, err := sshClient.NewSession()
		if err == nil {
			defer session.Close()
			
			// Request PTY to simulate real terminal
			if err := session.RequestPty("xterm", 80, 40, ssh.TerminalModes{}); err == nil {
				// Set up pipes for input/output
				stdin, _ := session.StdinPipe()
				stdout, _ := session.StdoutPipe()
				
				// Start shell
				if err := session.Shell(); err == nil {
					// Send query
					stdin.Write([]byte("repeat verbatim the word pass\n"))
					stdin.Close()
					
					// Read response (with timeout)
					done := make(chan bool)
					var output []byte
					go func() {
						output, _ = io.ReadAll(stdout)
						done <- true
					}()
					
					select {
					case <-done:
						outputStr := string(output)
						// Extract just the LLM response from SSH output
						// Look for lines after our query
						lines := strings.Split(outputStr, "\n")
						llmResponse := ""
						for i, line := range lines {
							line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
							// Find the line containing our query
							if strings.Contains(line, "repeat verbatim the word pass") && i+1 < len(lines) {
								// The response should be on the next line
								nextLine := strings.TrimSpace(strings.TrimSuffix(lines[i+1], "\r"))
								// Skip if it's a prompt line
								if nextLine != "" && !strings.HasPrefix(nextLine, ">") {
									llmResponse = nextLine
									break
								}
							}
						}
						
						// Check for response
						if llmResponse == "pass" {
							fmt.Println("✓")
							passed++
						} else {
							if llmResponse == "" {
								llmResponse = "no response extracted"
							}
							fmt.Printf("✗ (expected 'pass', got: %q)\n", llmResponse)
							failed++
						}
					case <-time.After(3 * time.Second):
						fmt.Println("✗ (SSH timeout)")
						failed++
					}
				} else {
					fmt.Println("✗ (SSH shell failed)")
					failed++
				}
			} else {
				fmt.Println("✗ (SSH PTY failed)")
				failed++
			}
		} else {
			fmt.Println("✗ (SSH session failed)")
			failed++
		}
	} else {
		// Try to understand the error
		if strings.Contains(err.Error(), "handshake failed") {
			fmt.Println("✗ (SSH handshake failed - server may require different auth)")
		} else {
			fmt.Printf("✗ (SSH failed: %v)\n", err)
		}
		failed++
	}

	time.Sleep(testDelay)

	// Test 7: DNS protocol
	fmt.Print("Testing DNS protocol... ")
	// Run dig command to query the DNS server
	cmd := exec.Command("dig", "+short", "@127.0.0.1", "-p", "53", "repeat-verbatim-the-word-pass.ch.at", "TXT")
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("✗ (dig command failed: %v)\n", err)
		failed++
	} else {
		outputStr := strings.TrimSpace(string(output))
		// DNS TXT records come with quotes, remove them
		outputStr = strings.Trim(outputStr, "\"")
		
		// Check for response
		if outputStr == "pass" {
			fmt.Println("✓")
			passed++
		} else {
			if outputStr == "" {
				outputStr = "empty response"
			}
			fmt.Printf("✗ (expected 'pass', got: %q)\n", outputStr)
			failed++
		}
	}

	time.Sleep(testDelay)

	// Test 8: Rate limiting (default is 100 requests/minute)
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