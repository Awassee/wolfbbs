package mailflow

import (
	"testing"

	"wolfbbs/internal/repository"
)

func TestTemplateLifecycle(t *testing.T) {
	repo := repository.NewInMemoryAdminRepository()
	rows, err := UpsertTemplate(repo, "Caller", Template{Name: "Follow-up", Subject: "Following up", Body: "Checking back in.", Urgency: "urgent"})
	if err != nil {
		t.Fatalf("upsert template: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 template, got %d", len(rows))
	}
	if rows[0].Urgency != "urgent" {
		t.Fatalf("expected urgency urgent, got %q", rows[0].Urgency)
	}
	loaded := LoadTemplates(repo, "caller")
	if len(loaded) != 1 || loaded[0].Name != "Follow-up" {
		t.Fatalf("unexpected loaded templates: %+v", loaded)
	}
	if err := RecordUse(repo, "caller", loaded[0].ID); err != nil {
		t.Fatalf("record use: %v", err)
	}
	loaded = LoadTemplates(repo, "caller")
	if loaded[0].UseCount != 1 {
		t.Fatalf("expected use count 1, got %+v", loaded[0])
	}
	_, err = DeleteTemplate(repo, "caller", loaded[0].ID)
	if err != nil {
		t.Fatalf("delete template: %v", err)
	}
	if got := LoadTemplates(repo, "caller"); len(got) != 0 {
		t.Fatalf("expected no templates after delete, got %+v", got)
	}
}

func TestTemplateNormalizationDropsInvalidRows(t *testing.T) {
	repo := repository.NewInMemoryAdminRepository()
	if err := repo.UpsertSystemSetting(SettingKey("caller"), `[{"id":"a","name":"","subject":"x","body":"y"},{"id":"b","name":"Kit","subject":"Hello","body":"Body","urgency":"bogus"}]`); err != nil {
		t.Fatalf("seed templates: %v", err)
	}
	rows := LoadTemplates(repo, "caller")
	if len(rows) != 1 {
		t.Fatalf("expected 1 valid template, got %+v", rows)
	}
	if rows[0].Urgency != "normal" {
		t.Fatalf("expected urgency normalized to normal, got %+v", rows[0])
	}
}
