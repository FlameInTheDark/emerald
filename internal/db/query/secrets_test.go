package query

import (
	"context"
	"strings"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

func TestSecretStoreCRUDAndTemplateValues(t *testing.T) {
	database, encryptor := newCompatTestDB(t)
	store := NewSecretStore(database.DB, encryptor)

	secret := &models.Secret{
		Name:  "db_password",
		Value: "super-secret",
	}
	if err := store.Create(context.Background(), secret); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if secret.ID == "" {
		t.Fatal("expected created secret id")
	}

	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 listed secret, got %d", len(listed))
	}
	if listed[0].Name != "db_password" {
		t.Fatalf("listed name = %q, want db_password", listed[0].Name)
	}
	if listed[0].Value != "" {
		t.Fatalf("expected listed secret value to be omitted, got %q", listed[0].Value)
	}

	loaded, err := store.GetByID(context.Background(), secret.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected stored secret")
	}
	if loaded.Name != "db_password" {
		t.Fatalf("loaded name = %q, want db_password", loaded.Name)
	}
	if loaded.Value != "" {
		t.Fatalf("expected GetByID secret value to be omitted, got %q", loaded.Value)
	}

	values, err := store.TemplateValues(context.Background())
	if err != nil {
		t.Fatalf("TemplateValues: %v", err)
	}
	if got := values["db_password"]; got != "super-secret" {
		t.Fatalf("template value = %q, want super-secret", got)
	}

	if err := store.Update(context.Background(), &models.Secret{
		ID:   secret.ID,
		Name: "primary_db_password",
	}, false); err != nil {
		t.Fatalf("Update rename: %v", err)
	}

	values, err = store.TemplateValues(context.Background())
	if err != nil {
		t.Fatalf("TemplateValues after rename: %v", err)
	}
	if _, exists := values["db_password"]; exists {
		t.Fatalf("expected old template key to be removed after rename: %#v", values)
	}
	if got := values["primary_db_password"]; got != "super-secret" {
		t.Fatalf("renamed template value = %q, want super-secret", got)
	}

	if err := store.Update(context.Background(), &models.Secret{
		ID:    secret.ID,
		Name:  "primary_db_password",
		Value: "rotated-secret",
	}, true); err != nil {
		t.Fatalf("Update rotate: %v", err)
	}

	values, err = store.TemplateValues(context.Background())
	if err != nil {
		t.Fatalf("TemplateValues after rotate: %v", err)
	}
	if got := values["primary_db_password"]; got != "rotated-secret" {
		t.Fatalf("rotated template value = %q, want rotated-secret", got)
	}

	if err := store.Delete(context.Background(), secret.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	values, err = store.TemplateValues(context.Background())
	if err != nil {
		t.Fatalf("TemplateValues after delete: %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("expected no template values after delete, got %#v", values)
	}
}

func TestSecretStoreRejectsInvalidSecretNames(t *testing.T) {
	database, encryptor := newCompatTestDB(t)
	store := NewSecretStore(database.DB, encryptor)

	invalidNames := []string{
		"",
		"db-password",
		"db.password",
		"contains space",
		"9starts_with_number",
	}

	for _, name := range invalidNames {
		name := name
		t.Run(name, func(t *testing.T) {
			err := store.Create(context.Background(), &models.Secret{
				Name:  name,
				Value: "value",
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), "name") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
