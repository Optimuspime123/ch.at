package main

import (
	"fmt"
	"os"
)

// Configuration - edit source code and recompile to change settings
// To disable a service: set its port to 0 or delete its .go file
const (
	HTTP_PORT  = 80  // Web interface (set to 0 to disable)
	HTTPS_PORT = 443 // TLS web interface (set to 0 to disable)
	SSH_PORT   = 22  // Anonymous SSH chat (set to 0 to disable)
	DNS_PORT   = 53  // DNS TXT chat (set to 0 to disable)
)


func main() {
	// SSH Server
	if SSH_PORT > 0 {
		go func() {
			if err := StartSSHServer(SSH_PORT); err != nil {
				fmt.Fprintf(os.Stderr, "SSH server error: %v\n", err)
			}
		}()
	}
	
	// DNS Server
	if DNS_PORT > 0 {
		go func() {
			if err := StartDNSServer(DNS_PORT); err != nil {
				fmt.Fprintf(os.Stderr, "DNS server error: %v\n", err)
			}
		}()
	}
	
	// HTTP/HTTPS Server
	// TODO: Implement graceful shutdown with signal handling
	if HTTP_PORT > 0 || HTTPS_PORT > 0 {
		if HTTPS_PORT > 0 {
			go func() {
				if err := StartHTTPSServer(HTTPS_PORT, "cert.pem", "key.pem"); err != nil {
					fmt.Fprintf(os.Stderr, "HTTPS server error: %v\n", err)
				}
			}()
		}
		
		if HTTP_PORT > 0 {
			if err := StartHTTPServer(HTTP_PORT); err != nil {
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