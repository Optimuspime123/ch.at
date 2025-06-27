package main

import (
	"fmt"
	"strings"

	"github.com/miekg/dns"
)

func StartDNSServer(port int) error {
	// Set up DNS handler
	dns.HandleFunc("ch.at.", handleDNS)
	dns.HandleFunc(".", handleDNS) // Catch-all for any domain

	// Create and start server
	server := &dns.Server{
		Addr: fmt.Sprintf(":%d", port),
		Net:  "udp",
	}

	fmt.Printf("DNS server listening on :%d\n", port)
	return server.ListenAndServe()
}

func handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	// Rate limiting
	if !rateLimitAllow(w.RemoteAddr().String()) {
		return // Silently drop - DNS doesn't have error responses for rate limits
	}

	// Check if we have a question
	if len(r.Question) == 0 {
		return
	}

	// Build response
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	// Process each question (usually just one)
	for _, q := range r.Question {
		if q.Qtype != dns.TypeTXT {
			continue // Only handle TXT queries
		}

		// Extract the prompt from domain name
		name := strings.TrimSuffix(strings.TrimSuffix(q.Name, "."), ".ch.at")
		prompt := strings.ReplaceAll(name, "-", " ")

		// Get LLM response
		response, err := LLM(prompt, nil)
		if err != nil {
			response = err.Error()
		}

		// Create TXT record
		txt := &dns.TXT{
			Hdr: dns.RR_Header{
				Name:   q.Name,
				Rrtype: dns.TypeTXT,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Txt: []string{response},
		}
		m.Answer = append(m.Answer, txt)
	}

	// Send response
	w.WriteMsg(m)
}