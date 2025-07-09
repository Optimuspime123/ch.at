package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func StartDNSServer(port int) error {
	dns.HandleFunc("ch.at.", handleDNS)
	dns.HandleFunc(".", handleDNS)

	server := &dns.Server{
		Addr: fmt.Sprintf(":%d", port),
		Net:  "udp",
	}

	return server.ListenAndServe()
}

func handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	if !rateLimitAllow(w.RemoteAddr().String()) {
		return
	}

	if len(r.Question) == 0 {
		return
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		if q.Qtype != dns.TypeTXT {
			continue
		}

		name := strings.TrimSuffix(strings.TrimSuffix(q.Name, "."), ".ch.at")
		prompt := strings.ReplaceAll(name, "-", " ")
		
		
		// Optimize prompt for DNS constraints
		dnsPrompt := "Answer in 500 characters or less, no markdown formatting: " + prompt

		// Stream LLM response with hard deadline
		ch := make(chan string)
		done := make(chan bool)
		
		go func() {
			if _, err := LLM(dnsPrompt, ch); err != nil {
				select {
				case ch <- "Error: " + err.Error():
				case <-done:
				}
			}
			// Don't close ch here - LLM function already does it with defer
		}()

		var response strings.Builder
		deadline := time.After(4 * time.Second) // Safe middle ground for DNS clients
		channelClosed := false
		
		
		for {
			select {
			case chunk, ok := <-ch:
				if !ok {
					channelClosed = true
					goto respond
				}
				response.WriteString(chunk)
				if response.Len() >= 500 {
					goto respond
				}
			case <-deadline:
				if response.Len() == 0 {
					response.WriteString("Request timed out")
				} else if !channelClosed {
					response.WriteString("... (incomplete)")
				}
				goto respond
			}
		}

	respond:
		close(done)
		finalResponse := response.String()
		if len(finalResponse) > 500 {
			finalResponse = finalResponse[:497] + "..."
		} else if len(finalResponse) == 500 && !channelClosed {
			// We hit the exact limit but stream is still going
			finalResponse = finalResponse[:497] + "..."
		}
		

		// Split response into 255-byte chunks for DNS TXT records
		var txtStrings []string
		for i := 0; i < len(finalResponse); i += 255 {
			end := i + 255
			if end > len(finalResponse) {
				end = len(finalResponse)
			}
			txtStrings = append(txtStrings, finalResponse[i:end])
		}
		
		txt := &dns.TXT{
			Hdr: dns.RR_Header{
				Name:   q.Name,
				Rrtype: dns.TypeTXT,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Txt: txtStrings,
		}
		m.Answer = append(m.Answer, txt)
	}

	w.WriteMsg(m)
}