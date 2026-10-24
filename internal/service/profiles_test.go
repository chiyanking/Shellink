package service

import (
	"errors"
	"path/filepath"
	"testing"

	"shellink/internal/protocol"
	"shellink/internal/store"
)

func TestProfileCreateUpdateAndSecretRedaction(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "shellink.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var key [32]byte
	key[0] = 7
	profiles := NewProfiles(db, key)
	created, err := profiles.Create(protocol.Object(
		"name", "bastion", "connectType", "command", "command", "expect jump.exp",
		"uniqueId", "  bastion-guid ", "password", "secret-value",
	))
	if err != nil {
		t.Fatal(err)
	}
	fields := protocol.MapValue(created)
	if fields["hasPassword"] != true || fields["uniqueId"] != "bastion-guid" {
		t.Fatalf("unexpected public profile: %#v", fields)
	}
	if _, leaks := fields["password"]; leaks {
		t.Fatal("public profile exposed its password")
	}
	id, _ := protocol.StringValue(fields["id"])
	stored, err := db.Profile(id)
	if err != nil || stored.EncryptedPassword == nil {
		t.Fatalf("password was not encrypted: %#v, %v", stored, err)
	}
	decrypted, err := store.NewSecretBox(key).Decrypt(*stored.EncryptedPassword)
	if err != nil || decrypted != "secret-value" {
		t.Fatalf("encrypted credential mismatch: %q, %v", decrypted, err)
	}

	updated, err := profiles.Update(id, protocol.Object("password", "", "name", "bastion-prod"))
	if err != nil {
		t.Fatal(err)
	}
	if protocol.MapValue(updated)["hasPassword"] != false || protocol.MapValue(updated)["name"] != "bastion-prod" {
		t.Fatalf("update did not clear credentials or change the name: %#v", updated)
	}
	if _, err := profiles.Create(protocol.Object("name", "duplicate", "connectType", "command", "command", "true", "uniqueId", "bastion-guid")); err == nil {
		t.Fatal("duplicate uniqueId was accepted")
	} else {
		var app *AppError
		if !errors.As(err, &app) || app.Status != 409 {
			t.Fatalf("expected a conflict, got %v", err)
		}
	}
}

func TestProfileRequiresCompleteTarget(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	profiles := NewProfiles(db, [32]byte{})
	if _, err := profiles.Create(protocol.Object("name", "missing-target")); err == nil {
		t.Fatal("incomplete SSH profile was accepted")
	}
}
