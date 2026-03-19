package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"wolfbbs/internal/domain"
)

const (
	sysSettingOperatorInsights = "analytics.operator_insights"
	maxOperatorInsightEvents   = 512
)

type operatorInsightEvent struct {
	Kind   string    `json:"kind"`
	Handle string    `json:"handle,omitempty"`
	Route  string    `json:"route,omitempty"`
	At     time.Time `json:"at"`
}

func normalizeOperatorInsightEvent(row operatorInsightEvent) (operatorInsightEvent, bool) {
	row.Kind = strings.ToLower(strings.TrimSpace(row.Kind))
	row.Handle = normalizeHandleKey(row.Handle)
	row.Route = strings.TrimSpace(row.Route)
	if row.At.IsZero() {
		row.At = time.Now().UTC()
	}
	if row.Kind == "" {
		return operatorInsightEvent{}, false
	}
	return row, true
}

func (a *webApp) loadOperatorInsightEvents() []operatorInsightEvent {
	if a == nil || a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingOperatorInsights)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	rows := []operatorInsightEvent{}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		a.addAppError("analytics.operator_insights", fmt.Errorf("decode operator insights: %w", err))
		return nil
	}
	out := make([]operatorInsightEvent, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeOperatorInsightEvent(row); ok {
			out = append(out, normalized)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

func (a *webApp) persistOperatorInsightEvents(rows []operatorInsightEvent) {
	if a == nil || a.adminRepo == nil {
		return
	}
	clean := make([]operatorInsightEvent, 0, len(rows))
	for _, row := range rows {
		if normalized, ok := normalizeOperatorInsightEvent(row); ok {
			clean = append(clean, normalized)
		}
	}
	sort.Slice(clean, func(i, j int) bool { return clean[i].At.After(clean[j].At) })
	if len(clean) > maxOperatorInsightEvents {
		clean = clean[:maxOperatorInsightEvents]
	}
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			a.addAppError("analytics.operator_insights", fmt.Errorf("encode operator insights: %w", err))
			return
		}
		body = string(raw)
	}
	a.persistSystemSetting(sysSettingOperatorInsights, body)
}

func (a *webApp) recordOperatorInsight(kind, handle, route string) {
	if a == nil || a.adminRepo == nil {
		return
	}
	row, ok := normalizeOperatorInsightEvent(operatorInsightEvent{
		Kind:   kind,
		Handle: handle,
		Route:  route,
		At:     time.Now().UTC(),
	})
	if !ok {
		return
	}
	rows := a.loadOperatorInsightEvents()
	rows = append([]operatorInsightEvent{row}, rows...)
	a.persistOperatorInsightEvents(rows)
}

func firstCallTaskCount(snapshot firstCallSnapshot) int {
	done := 0
	for _, task := range snapshot.Tasks {
		if task.Done {
			done++
		}
	}
	return done
}

func (a *webApp) recordFirstCallTransition(user *domain.User, before firstCallSnapshot, route string) {
	if a == nil || user == nil {
		return
	}
	after := a.buildFirstCallSnapshot(user)
	if firstCallTaskCount(after) == len(after.Tasks) && firstCallTaskCount(before) < len(before.Tasks) {
		a.recordOperatorInsight("first_call.complete", user.Handle, route)
	}
}
