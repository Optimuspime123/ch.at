package main

import (
	"context"
	"fmt"
	"net"
	"strings"
)

type DNSServer struct {
	port int
}

func NewDNSServer(port int) *DNSServer {
	return &DNSServer{port: port}
}

func (s *DNSServer) Start() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Printf("DNS server listening on :%d\n", s.port)

	buf := make([]byte, 512) // DNS messages are typically small
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Read error - continue
			continue
		}

		go s.handleQuery(conn, clientAddr, buf[:n])
	}
}

func (s *DNSServer) handleQuery(conn *net.UDPConn, addr *net.UDPAddr, query []byte) {
	// Validate minimum DNS packet size
	if len(query) < 12 {
		return
	}

	// Rate limiting
	if !rateLimiter.Allow(addr.String()) {
		return // Silently drop - DNS doesn't have error responses for rate limits
	}

	// Validate DNS header flags (must be a query, not response)
	if query[2]&0x80 != 0 {
		return // It's a response, not a query
	}

	// Extract question from query
	question := extractQuestion(query)
	if question == "" {
		return
	}

	// Remove .ch.at suffix if present
	question = strings.TrimSuffix(question, ".ch.at")
	question = strings.TrimSuffix(question, ".")
	
	// Convert DNS format to readable (replace - with space)
	prompt := strings.ReplaceAll(question, "-", " ")

	// Get LLM response
	ctx := context.Background()
	response, err := getLLMResponse(ctx, prompt)
	if err != nil {
		response = "Error: " + err.Error()
	}

	// Build DNS response with chunked TXT records
	reply := buildDNSResponse(query, response)
	
	// Ensure response fits in UDP packet (RFC recommends 512 bytes)
	if len(reply) > 512 {
		// Truncate and set TC bit
		reply = reply[:512]
		reply[2] |= 0x02 // Set TC (truncation) bit
	}
	
	conn.WriteToUDP(reply, addr)
}

func extractQuestion(query []byte) string {
	// Skip header (12 bytes)
	if len(query) < 12 {
		return ""
	}
	
	pos := 12
	var name []string
	totalLength := 0
	
	// Parse domain name labels (max 128 to prevent DoS)
	for i := 0; i < 128 && pos < len(query); i++ {
		if pos >= len(query) {
			return ""
		}
		
		length := int(query[pos])
		if length == 0 {
			break
		}
		
		// DNS compression uses first 2 bits = 11 (0xC0)
		// We reject these for simplicity and security
		if length&0xC0 == 0xC0 {
			return ""
		}
		
		// DNS label length must be <= 63
		if length > 63 {
			return ""
		}
		
		pos++
		if pos+length > len(query) {
			return ""
		}
		
		// Track total domain name length (max 255)
		totalLength += length + 1
		if totalLength > 255 {
			return ""
		}
		
		// Validate label contains reasonable characters
		label := query[pos : pos+length]
		name = append(name, string(label))
		pos += length
	}
	
	// Ensure we read a complete question (should have type and class after)
	if pos+4 > len(query) {
		return ""
	}
	
	return strings.Join(name, ".")
}

func buildDNSResponse(query []byte, answer string) []byte {
	resp := make([]byte, len(query))
	copy(resp, query)
	
	// Set response flags (QR=1, AA=1)
	resp[2] = 0x81
	resp[3] = 0x80
	
	// Set answer count to 1
	resp[7] = 1
	
	// Skip to end of question section
	pos := 12
	for pos < len(resp) {
		if resp[pos] == 0 {
			pos += 5 // Skip null terminator + type + class
			break
		}
		pos++
	}
	
	// Add answer section
	// Pointer to question name
	resp = append(resp, 0xc0, 0x0c)
	
	// Type TXT (16), Class IN (1)
	resp = append(resp, 0x00, 0x10, 0x00, 0x01)
	
	// TTL (0)
	resp = append(resp, 0x00, 0x00, 0x00, 0x00)
	
	// Build TXT record data with chunking
	txtData := buildTXTData(answer)
	
	// Data length
	resp = append(resp, byte(len(txtData)>>8), byte(len(txtData)))
	
	// TXT data
	resp = append(resp, txtData...)
	
	return resp
}

func buildTXTData(text string) []byte {
	var data []byte
	
	// Split into 255-byte chunks
	for len(text) > 0 {
		chunkLen := len(text)
		if chunkLen > 255 {
			chunkLen = 255
		}
		
		data = append(data, byte(chunkLen))
		data = append(data, text[:chunkLen]...)
		text = text[chunkLen:]
	}
	
	return data
}