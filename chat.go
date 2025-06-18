package main

import (
	"fmt"
	"os"
)

// Configuration - edit source code and recompile to change settings
// To disable a service: set its port to 0 or delete its .go file
const (
	HTTP_PORT   = 80    // Web interface (set to 0 to disable)
	HTTPS_PORT  = 443   // TLS web interface (set to 0 to disable)
	SSH_PORT    = 22    // Anonymous SSH chat (set to 0 to disable)
	DNS_PORT    = 53    // DNS TXT chat (set to 0 to disable)
	OPENAI_PORT = 8080  // OpenAI-compatible API (set to 0 to disable)
	CERT_FILE   = "cert.pem" // TLS certificate for HTTPS
	KEY_FILE    = "key.pem"  // TLS key for HTTPS
)

func main() {
	// SSH Server
	if SSH_PORT > 0 {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "SSH server panic: %v\n", r)
				}
			}()
			sshServer := NewSSHServer(SSH_PORT)
			if err := sshServer.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "SSH server error: %v\n", err)
			}
		}()
	}
	
	// DNS Server
	if DNS_PORT > 0 {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "DNS server panic: %v\n", r)
				}
			}()
			dnsServer := NewDNSServer(DNS_PORT)
			if err := dnsServer.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "DNS server error: %v\n", err)
			}
		}()
	}
	
	// OpenAI API Server
	if OPENAI_PORT > 0 {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "OpenAI API server panic: %v\n", r)
				}
			}()
			openaiServer := NewOpenAIServer(OPENAI_PORT)
			if err := openaiServer.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "OpenAI API server error: %v\n", err)
			}
		}()
	}
	
	// HTTP/HTTPS Server
	// TODO: Implement graceful shutdown with signal handling
	if HTTP_PORT > 0 || HTTPS_PORT > 0 {
		httpServer := NewHTTPServer(HTTP_PORT)
		
		if HTTPS_PORT > 0 {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Fprintf(os.Stderr, "HTTPS server panic: %v\n", r)
					}
				}()
				if err := httpServer.StartTLS(HTTPS_PORT, CERT_FILE, KEY_FILE); err != nil {
					fmt.Fprintf(os.Stderr, "HTTPS server error: %v\n", err)
				}
			}()
		}
		
		if HTTP_PORT > 0 {
			if err := httpServer.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
				os.Exit(1)
			}
		} else {
			// If only HTTPS is enabled, block forever
			select {}
		}
	} else {
		// If no servers enabled, block forever
		select {}
	}
}