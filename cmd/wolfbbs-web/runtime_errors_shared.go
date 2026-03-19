package main

import (
	"encoding/json"
	"strings"
	"time"
)

const sysSettingSharedRuntimeErrors = "runtime.errors.shared"

func normalizeSharedRuntimeErrorRows(rows []appErrorEntry) []appErrorEntry {
	out := make([]appErrorEntry, 0, len(rows))
	for _, row := range rows {
		row.Time = row.Time.UTC()
		row.Area = cleanOneLiner(defaultIfBlank(row.Area, "runtime"), 64)
		row.Message = cleanOneLiner(row.Message, 240)
		if row.Time.IsZero() || row.Message == "" {
			continue
		}
		out = append(out, row)
	}
	if len(out) > maxAdminErrorEntries {
		out = out[len(out)-maxAdminErrorEntries:]
	}
	return out
}

func (a *webApp) loadSharedRuntimeErrors() []appErrorEntry {
	if a == nil || a.adminRepo == nil {
		return nil
	}
	raw, err := a.adminRepo.GetSystemSetting(sysSettingSharedRuntimeErrors)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []appErrorEntry
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	return normalizeSharedRuntimeErrorRows(rows)
}

func (a *webApp) persistSharedRuntimeErrors(rows []appErrorEntry) {
	if a == nil || a.adminRepo == nil {
		return
	}
	clean := normalizeSharedRuntimeErrorRows(rows)
	body := ""
	if len(clean) > 0 {
		raw, err := json.Marshal(clean)
		if err != nil {
			return
		}
		body = string(raw)
	}
	_ = a.adminRepo.UpsertSystemSetting(sysSettingSharedRuntimeErrors, body)
}

func (a *webApp) appendSharedRuntimeError(entry appErrorEntry) {
	if a == nil || a.adminRepo == nil {
		return
	}
	entry.Time = entry.Time.UTC()
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	rows := a.loadSharedRuntimeErrors()
	rows = append(rows, entry)
	a.persistSharedRuntimeErrors(rows)
}

func reverseSharedRuntimeErrors(rows []appErrorEntry, limit int) []appErrorEntry {
	if limit <= 0 {
		limit = 50
	}
	if len(rows) == 0 {
		return nil
	}
	if limit > len(rows) {
		limit = len(rows)
	}
	out := make([]appErrorEntry, 0, limit)
	for i := len(rows) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, rows[i])
	}
	return out
}
