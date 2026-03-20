package mailflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"wolfbbs/internal/repository"
)

const TemplateSettingRoot = "mail.templates."

type Template struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Urgency   string    `json:"urgency,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	UseCount  int       `json:"use_count,omitempty"`
}

func SettingKey(handle string) string {
	handle = normalizeHandle(handle)
	if handle == "" {
		return ""
	}
	return TemplateSettingRoot + handle
}

func LoadTemplates(repo repository.AdminRepository, handle string) []Template {
	key := SettingKey(handle)
	if repo == nil || key == "" {
		return nil
	}
	raw, err := repo.GetSystemSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []Template
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	return normalizeTemplates(rows)
}

func SaveTemplates(repo repository.AdminRepository, handle string, rows []Template) error {
	key := SettingKey(handle)
	if repo == nil || key == "" {
		return nil
	}
	rows = normalizeTemplates(rows)
	body := ""
	if len(rows) > 0 {
		raw, err := json.Marshal(rows)
		if err != nil {
			return fmt.Errorf("encode templates: %w", err)
		}
		body = string(raw)
	}
	return repo.UpsertSystemSetting(key, body)
}

func UpsertTemplate(repo repository.AdminRepository, handle string, row Template) ([]Template, error) {
	rows := LoadTemplates(repo, handle)
	row.ID = strings.TrimSpace(row.ID)
	if row.ID == "" {
		row.ID = fmt.Sprintf("tpl-%d", time.Now().UTC().UnixNano())
	}
	row.Name = cleanOneLine(row.Name, 48)
	row.Subject = cleanOneLine(row.Subject, 120)
	row.Body = cleanBody(row.Body, 4096)
	row.Urgency = normalizeUrgency(row.Urgency)
	if row.Name == "" || row.Subject == "" || row.Body == "" {
		return rows, fmt.Errorf("name, subject, and body are required")
	}
	row.UpdatedAt = time.Now().UTC()
	updated := false
	for i := range rows {
		if rows[i].ID == row.ID {
			row.UseCount = rows[i].UseCount
			rows[i] = row
			updated = true
			break
		}
	}
	if !updated {
		rows = append(rows, row)
	}
	if err := SaveTemplates(repo, handle, rows); err != nil {
		return rows, err
	}
	return rows, nil
}

func DeleteTemplate(repo repository.AdminRepository, handle, id string) ([]Template, error) {
	rows := LoadTemplates(repo, handle)
	id = strings.TrimSpace(id)
	if id == "" {
		return rows, fmt.Errorf("template id is required")
	}
	filtered := make([]Template, 0, len(rows))
	for _, row := range rows {
		if row.ID == id {
			continue
		}
		filtered = append(filtered, row)
	}
	if len(filtered) == len(rows) {
		return rows, fmt.Errorf("template not found")
	}
	if err := SaveTemplates(repo, handle, filtered); err != nil {
		return rows, err
	}
	return filtered, nil
}

func FindTemplate(rows []Template, id string) (Template, bool) {
	id = strings.TrimSpace(id)
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return Template{}, false
}

func RecordUse(repo repository.AdminRepository, handle, id string) error {
	rows := LoadTemplates(repo, handle)
	for i := range rows {
		if rows[i].ID != strings.TrimSpace(id) {
			continue
		}
		rows[i].UseCount++
		rows[i].UpdatedAt = time.Now().UTC()
		return SaveTemplates(repo, handle, rows)
	}
	return nil
}

func normalizeTemplates(rows []Template) []Template {
	out := make([]Template, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		row.ID = strings.TrimSpace(row.ID)
		row.Name = cleanOneLine(row.Name, 48)
		row.Subject = cleanOneLine(row.Subject, 120)
		row.Body = cleanBody(row.Body, 4096)
		row.Urgency = normalizeUrgency(row.Urgency)
		if row.ID == "" || row.Name == "" || row.Subject == "" || row.Body == "" {
			continue
		}
		if _, ok := seen[row.ID]; ok {
			continue
		}
		seen[row.ID] = struct{}{}
		if row.UpdatedAt.IsZero() {
			row.UpdatedAt = time.Now().UTC()
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UseCount != out[j].UseCount {
			return out[i].UseCount > out[j].UseCount
		}
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func normalizeHandle(handle string) string {
	return strings.ToLower(strings.TrimSpace(handle))
}

func normalizeUrgency(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "normal", "fyi", "urgent", "asap":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "normal"
	}
}

func cleanOneLine(value string, max int) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " "))
	if max > 0 && len(value) > max {
		return strings.TrimSpace(value[:max])
	}
	return value
}

func cleanBody(value string, max int) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.TrimSpace(value)
	if max > 0 && len(value) > max {
		value = strings.TrimSpace(value[:max])
	}
	return value
}
