package main

import (
	"net/http"
	"strings"
	"time"
)

const maxAdminCredentialReceipts = 4

type adminCredentialReceipt struct {
	Action   string    `json:"action"`
	Handle   string    `json:"handle"`
	Password string    `json:"password"`
	IssuedAt time.Time `json:"issued_at"`
}

func normalizeAdminCredentialReceipt(row adminCredentialReceipt) (adminCredentialReceipt, bool) {
	row.Action = strings.ToLower(strings.TrimSpace(row.Action))
	row.Handle = strings.TrimSpace(row.Handle)
	row.Password = strings.TrimSpace(row.Password)
	if row.IssuedAt.IsZero() {
		row.IssuedAt = time.Now().UTC()
	}
	switch row.Action {
	case "create", "reset":
	default:
		return adminCredentialReceipt{}, false
	}
	if row.Handle == "" || row.Password == "" {
		return adminCredentialReceipt{}, false
	}
	return row, true
}

func (a *webApp) queueAdminCredentialReceipt(r *http.Request, row adminCredentialReceipt) bool {
	if a == nil || r == nil {
		return false
	}
	normalized, ok := normalizeAdminCredentialReceipt(row)
	if !ok {
		return false
	}
	cookie, err := r.Cookie("wolfbbs_session")
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false
	}
	a.Lock()
	defer a.Unlock()
	state, exists := a.sessions[cookie.Value]
	if !exists || time.Now().After(state.expire) {
		delete(a.sessions, cookie.Value)
		return false
	}
	state.credentialReceipts = append([]adminCredentialReceipt{normalized}, state.credentialReceipts...)
	if len(state.credentialReceipts) > maxAdminCredentialReceipts {
		state.credentialReceipts = state.credentialReceipts[:maxAdminCredentialReceipts]
	}
	a.sessions[cookie.Value] = state
	return true
}

func (a *webApp) popAdminCredentialReceipts(r *http.Request) []adminCredentialReceipt {
	if a == nil || r == nil {
		return nil
	}
	cookie, err := r.Cookie("wolfbbs_session")
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil
	}
	a.Lock()
	defer a.Unlock()
	state, exists := a.sessions[cookie.Value]
	if !exists || time.Now().After(state.expire) {
		delete(a.sessions, cookie.Value)
		return nil
	}
	if len(state.credentialReceipts) == 0 {
		return nil
	}
	out := make([]adminCredentialReceipt, len(state.credentialReceipts))
	copy(out, state.credentialReceipts)
	state.credentialReceipts = nil
	a.sessions[cookie.Value] = state
	return out
}

func renderAdminCredentialReceiptBlock(rows []adminCredentialReceipt) string {
	if len(rows) == 0 {
		return ""
	}
	out := strings.Builder{}
	out.WriteString(`<section class="wolfbbs-card"><h2>One-time credential receipts</h2><p class="wolfbbs-muted">These credentials are only shown once on the next page load. Save them before navigating away.</p><div class="wolfbbs-helper-grid">`)
	for _, row := range rows {
		verb := "Created account"
		if row.Action == "reset" {
			verb = "Reset password"
		}
		out.WriteString(`<article class="wolfbbs-helper-card"><strong>` + htmlEscape(verb) + `</strong><p><strong>Handle:</strong> <code>` + htmlEscape(row.Handle) + `</code><br><strong>Temporary password:</strong> <code>` + htmlEscape(row.Password) + `</code></p><p class="wolfbbs-muted">Issued ` + htmlEscape(row.IssuedAt.Local().Format("2006-01-02 15:04:05")) + `.</p></article>`)
	}
	out.WriteString(`</div></section>`)
	return out.String()
}
