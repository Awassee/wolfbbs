package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"

	"wolfbbs/internal/logging"
)

type ingestPayload struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	RawHeaders string `json:"raw_headers"`
}

func main() {
	logging.ConfigureStdLogger("wolfbbs-mailin")
	listen := flag.String("listen", ":8091", "mail ingest listen address")
	forwardURL := flag.String("forward-url", strings.TrimSpace(os.Getenv("WOLFBBS_MAILIN_FORWARD_URL")), "WolfBBS inbound endpoint URL")
	token := flag.String("token", strings.TrimSpace(os.Getenv("WOLFBBS_INBOUND_TOKEN")), "shared inbound token")
	allowDomains := flag.String("allow-domains", strings.TrimSpace(os.Getenv("WOLFBBS_MAILIN_ALLOW_DOMAINS")), "comma-separated sender domains")
	flag.Parse()

	if *forwardURL == "" {
		*forwardURL = "http://web:8080/mail/inbound"
	}
	if strings.TrimSpace(*token) == "" {
		log.Fatal("missing inbound token; set WOLFBBS_INBOUND_TOKEN")
	}

	allowed := parseAllowDomains(*allowDomains)
	client := &http.Client{Timeout: 10 * time.Second}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_ = r
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	http.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var payload ingestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		payload.From = strings.TrimSpace(payload.From)
		payload.To = strings.TrimSpace(payload.To)
		payload.Subject = strings.TrimSpace(payload.Subject)
		payload.Body = strings.TrimSpace(payload.Body)
		if payload.From == "" || payload.To == "" || payload.Subject == "" || payload.Body == "" {
			http.Error(w, "from/to/subject/body required", http.StatusBadRequest)
			return
		}
		if len(allowed) > 0 {
			domain := senderDomain(payload.From)
			if domain == "" {
				http.Error(w, "invalid sender", http.StatusBadRequest)
				return
			}
			if _, ok := allowed[domain]; !ok {
				http.Error(w, "sender domain blocked", http.StatusForbidden)
				return
			}
		}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequest(http.MethodPost, *forwardURL, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "forward request build failed", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Inbound-Token", *token)
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "forward failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			http.Error(w, fmt.Sprintf("forward rejected: %s", strings.TrimSpace(string(data))), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted"))
	})

	log.Printf("wolfbbs-mailin listening on %s forwarding to %s", *listen, *forwardURL)
	log.Fatal(http.ListenAndServe(*listen, nil))
}

func parseAllowDomains(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		out[part] = struct{}{}
	}
	return out
}

func senderDomain(from string) string {
	from = strings.TrimSpace(from)
	if from == "" {
		return ""
	}
	if strings.Contains(from, "<") {
		if addr, err := mail.ParseAddress(from); err == nil {
			from = addr.Address
		}
	}
	at := strings.LastIndex(from, "@")
	if at <= 0 || at+1 >= len(from) {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(from[at+1:]))
}
