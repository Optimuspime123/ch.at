package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

type SSHServer struct {
	port int
}

func NewSSHServer(port int) *SSHServer {
	return &SSHServer{port: port}
}

func (s *SSHServer) Start() error {
	// SSH server configuration
	config := &ssh.ServerConfig{
		NoClientAuth: true, // Anonymous access
	}

	// Get or create persistent host key
	privateKey, err := getOrCreateHostKey()
	if err != nil {
		return fmt.Errorf("failed to get host key: %v", err)
	}
	config.AddHostKey(privateKey)

	// Listen for connections
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
	}
	defer listener.Close()

	fmt.Printf("SSH server listening on :%d\n", s.port)

	// Simple connection limiting
	sem := make(chan struct{}, 100) // Max 100 concurrent SSH connections
	
	for {
		conn, err := listener.Accept()
		if err != nil {
			// Connection error - continue accepting others
			continue
		}
		
		select {
		case sem <- struct{}{}:
			go func() {
				defer func() { <-sem }()
				s.handleConnection(conn, config)
			}()
		default:
			// Too many connections
			conn.Close()
		}
	}
}

func (s *SSHServer) handleConnection(netConn net.Conn, config *ssh.ServerConfig) {
	defer netConn.Close()

	// Rate limiting
	if !rateLimiter.Allow(netConn.RemoteAddr().String()) {
		netConn.Write([]byte("Rate limit exceeded. Please try again later.\r\n"))
		return
	}

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(netConn, config)
	if err != nil {
		// Handshake failed - continue accepting others
		return
	}
	defer sshConn.Close()

	go ssh.DiscardRequests(reqs)

	// Handle channels (sessions)
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			// Channel error - continue
			continue
		}

		go s.handleSession(channel, requests)
	}
}

func (s *SSHServer) handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()

	// Handle session requests
	go func() {
		for req := range requests {
			switch req.Type {
			case "shell", "pty-req":
				req.Reply(true, nil)
			default:
				req.Reply(false, nil)
			}
		}
	}()

	fmt.Fprintf(channel, "Welcome to ch.at\r\n")
	fmt.Fprintf(channel, "Type your message and press Enter. Type 'exit' to quit.\r\n")
	fmt.Fprintf(channel, "> ")

	// Read line by line
	var input strings.Builder
	buf := make([]byte, 1024)

	for {
		n, err := channel.Read(buf)
		if err != nil {
			if err != io.EOF {
				// Read error - exit session
			}
			return
		}

		data := string(buf[:n])
		for _, ch := range data {
			if ch == '\n' || ch == '\r' {
				if input.Len() > 0 {
					query := strings.TrimSpace(input.String())
					input.Reset()

					if query == "exit" {
						fmt.Fprintf(channel, "Goodbye!\r\n")
						return
					}

					// Get LLM response with streaming
					ctx := context.Background()
					stream, err := getLLMResponseStream(ctx, query)
					if err != nil {
						fmt.Fprintf(channel, "Error: %v\r\n", err)
						fmt.Fprintf(channel, "> ")
						continue
					}
					
					// Stream response as it arrives
					for chunk := range stream {
						fmt.Fprint(channel, chunk)
						if f, ok := channel.(interface{ Flush() }); ok {
							f.Flush()
						}
					}
					fmt.Fprintf(channel, "\r\n> ")
				}
			} else {
				input.WriteRune(ch)
			}
		}
	}
}

// getOrCreateHostKey loads existing key or generates new one
func getOrCreateHostKey() (ssh.Signer, error) {
	keyPath := "ssh_host_key"
	
	// Try to load existing key
	if keyData, err := os.ReadFile(keyPath); err == nil {
		return ssh.ParsePrivateKey(keyData)
	}

	// Generate new ephemeral key (more private but less convenient)
	// Users will see "host key changed" warnings on each restart
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	// Optionally save for convenience (comment out for max privacy)
	keyData := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		// Couldn't save host key - continue anyway
	}

	return ssh.NewSignerFromKey(key)
}