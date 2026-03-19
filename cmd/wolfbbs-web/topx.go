package main

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type topXRow struct {
	Handle    string
	Posts     int
	ChatLines int
	DoorRuns  int
	Champions int
	Momentum  int
}

func (a *webApp) handleTopX(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rangeKey := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("range")))
	if rangeKey == "" {
		rangeKey = "week"
	}
	var since time.Time
	switch rangeKey {
	case "all":
		since = time.Time{}
	case "month":
		since = time.Now().UTC().AddDate(0, 0, -30)
	default:
		rangeKey = "week"
		since = time.Now().UTC().AddDate(0, 0, -7)
	}
	rows := a.buildTopXRows(since)
	rangeLabel := map[string]string{
		"week":  "Weekly",
		"month": "Monthly",
		"all":   "All-Time",
	}[rangeKey]
	dataRows := strings.Builder{}
	for idx, row := range rows {
		dataRows.WriteString(`<tr><td>` + strconv.Itoa(idx+1) + `</td><td>` + htmlEscape(row.Handle) + `</td><td>` + strconv.Itoa(row.Posts) + `</td><td>` + strconv.Itoa(row.ChatLines) + `</td><td>` + strconv.Itoa(row.DoorRuns) + `</td><td>` + strconv.Itoa(row.Champions) + `</td><td>` + strconv.Itoa(row.Momentum) + `</td></tr>`)
	}
	if dataRows.Len() == 0 {
		dataRows.WriteString(`<tr><td colspan="7">No activity in this range yet.</td></tr>`)
	}
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>TopX Leaderboards</title></head><body>
<h1>TopX Leaderboards</h1>
<p><a href="/start">start</a> | <a href="/today">today</a> | <a href="/boards">boards</a> | <a href="/chat">chat</a> | <a href="/doors">doors</a> | <a href="/scores">scores</a> | <a href="/topx">topx</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
<p><strong>Caller:</strong> ` + htmlEscape(user.Handle) + `</p>
<form method="GET" action="/topx" class="wolfbbs-inline-form"><label>Range <select name="range"><option value="week"` + selectedIf(rangeKey == "week") + `>Weekly</option><option value="month"` + selectedIf(rangeKey == "month") + `>Monthly</option><option value="all"` + selectedIf(rangeKey == "all") + `>All-Time</option></select></label><button type="submit">Apply</button></form>
<p>Momentum = posts + live chat + door play + champion streak in one board.</p>
<h2>` + htmlEscape(rangeLabel) + ` TopX</h2>
<table border="1"><tr><th>#</th><th>Handle</th><th>Posts</th><th>Chat</th><th>Door Runs</th><th>Champions</th><th>Momentum</th></tr>` + dataRows.String() + `</table>
</body></html>`
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

func (a *webApp) buildTopXRows(since time.Time) []topXRow {
	idToHandle := map[int64]string{}
	if a.authSvc != nil {
		if users, err := a.authSvc.ListUsers(); err == nil {
			for _, row := range users {
				idToHandle[row.ID] = row.Handle
			}
		}
	}
	findHandle := func(userID int64) string {
		if handle := strings.TrimSpace(idToHandle[userID]); handle != "" {
			return handle
		}
		return "uid:" + strconv.FormatInt(userID, 10)
	}

	rows := map[string]*topXRow{}
	ensure := func(handle string) *topXRow {
		handle = strings.TrimSpace(handle)
		if handle == "" {
			handle = "(unknown)"
		}
		row, ok := rows[handle]
		if !ok {
			row = &topXRow{Handle: handle}
			rows[handle] = row
		}
		return row
	}
	includeTime := func(ts time.Time) bool {
		if since.IsZero() {
			return true
		}
		if ts.IsZero() {
			return false
		}
		return !ts.Before(since)
	}

	if a.boardRepo != nil && a.msgRepo != nil {
		if boards, err := a.boardRepo.List(); err == nil {
			for _, board := range boards {
				msgs, err := a.msgRepo.ListByBoard(board.ID)
				if err != nil {
					continue
				}
				for _, msg := range msgs {
					if !includeTime(msg.CreatedAt) {
						continue
					}
					ensure(findHandle(msg.AuthorID)).Posts++
				}
			}
		}
	}
	if a.chatSvc != nil {
		for _, channel := range a.chatSvc.ListChannels() {
			for _, msg := range a.chatSvc.History(channel, 500) {
				if !includeTime(msg.CreatedAt) {
					continue
				}
				ensure(msg.From).ChatLines++
			}
		}
	}
	if a.doorRegistry != nil {
		for _, door := range a.doorRegistry.Doors() {
			scores, err := a.doorRegistry.ListScores(door.ID, 100)
			if err != nil || len(scores) == 0 {
				continue
			}
			top := scores[0]
			if includeTime(top.CreatedAt) {
				ensure(findHandle(top.UserID)).Champions++
			}
			for _, score := range scores {
				if !includeTime(score.CreatedAt) {
					continue
				}
				ensure(findHandle(score.UserID)).DoorRuns++
			}
		}
	}

	out := make([]topXRow, 0, len(rows))
	for _, row := range rows {
		row.Momentum = row.Posts*5 + row.ChatLines*2 + row.DoorRuns*3 + row.Champions*8
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Momentum == out[j].Momentum {
			return strings.ToLower(out[i].Handle) < strings.ToLower(out[j].Handle)
		}
		return out[i].Momentum > out[j].Momentum
	})
	if len(out) > 25 {
		out = out[:25]
	}
	return out
}
