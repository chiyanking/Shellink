package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"shellink/internal/protocol"
	"shellink/internal/store"
)

type Profiles struct {
	store *store.Store
	box   store.SecretBox
}

func NewProfiles(db *store.Store, masterKey [32]byte) *Profiles {
	return &Profiles{store: db, box: store.NewSecretBox(masterKey)}
}

func (s *Profiles) List(params protocol.Value) (protocol.Value, error) {
	query := ""
	if value, ok := protocol.MapValue(params)["q"]; ok {
		var valid bool
		query, valid = protocol.StringValue(value)
		if !valid {
			return nil, Invalid("q must be a string")
		}
	}
	profiles, err := s.store.Profiles(query)
	if err != nil {
		return nil, Internal(err)
	}
	values := make([]protocol.Value, 0, len(profiles))
	for _, profile := range profiles {
		values = append(values, publicProfile(profile))
	}
	return values, nil
}

func (s *Profiles) Get(id string) (protocol.Value, error) {
	profile, err := s.store.Profile(id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, NotFound("Profile not found")
	}
	if err != nil {
		return nil, Internal(err)
	}
	return publicProfile(profile), nil
}

func (s *Profiles) Create(params protocol.Value) (protocol.Value, error) {
	fields := protocol.MapValue(params)
	name, ok := optionalString(fields, "name")
	if !ok || name == "" {
		return nil, Invalid("name must be a non-empty string")
	}
	profile := store.Profile{
		ID: randomUUID(), Name: name, ConnectType: "ssh", Host: "", Port: 22, Username: "",
		AuthType: "password", Term: "xterm-256color", Cols: 160, Rows: 42,
		CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli(),
	}
	if connectType, present := fields["connectType"]; present {
		value, valid := protocol.StringValue(connectType)
		if !valid || (value != "ssh" && value != "command") {
			return nil, Invalid("connectType must be ssh or command")
		}
		profile.ConnectType = value
	}
	if err := setOptionalString(fields, "command", &profile.Command); err != nil {
		return nil, err
	}
	if err := setString(fields, "host", &profile.Host); err != nil {
		return nil, err
	}
	if err := setString(fields, "username", &profile.Username); err != nil {
		return nil, err
	}
	if err := setString(fields, "term", &profile.Term); err != nil {
		return nil, err
	}
	if err := setOptionalString(fields, "promptRegex", &profile.PromptRegex); err != nil {
		return nil, err
	}
	if err := setOptionalString(fields, "uniqueId", &profile.UniqueID); err != nil {
		return nil, err
	}
	if err := setString(fields, "authType", &profile.AuthType); err != nil {
		return nil, err
	}
	if profile.AuthType != "password" && profile.AuthType != "key" {
		return nil, Invalid("authType must be password or key")
	}
	if err := setInteger(fields, "port", &profile.Port, 1, 65535); err != nil {
		return nil, err
	}
	if err := setInteger(fields, "cols", &profile.Cols, 20, 500); err != nil {
		return nil, err
	}
	if err := setInteger(fields, "rows", &profile.Rows, 5, 200); err != nil {
		return nil, err
	}
	if err := validateTarget(profile); err != nil {
		return nil, err
	}
	if profile.UniqueID != nil {
		trimmed := strings.TrimSpace(*profile.UniqueID)
		if trimmed == "" {
			profile.UniqueID = nil
		} else {
			profile.UniqueID = ptr(trimmed)
		}
	}
	if err := s.assertUniqueID(profile.UniqueID, ""); err != nil {
		return nil, err
	}
	for inputName, target := range map[string]**string{
		"password": &profile.EncryptedPassword, "privateKey": &profile.EncryptedPrivateKey, "passphrase": &profile.EncryptedPassphrase,
	} {
		if raw, present := fields[inputName]; present && raw != nil {
			secret, valid := protocol.StringValue(raw)
			if !valid {
				return nil, Invalid(inputName + " must be a string")
			}
			if secret != "" {
				encrypted, err := s.box.Encrypt(secret)
				if err != nil {
					return nil, Internal(err)
				}
				*target = ptr(encrypted)
			}
		}
	}
	if err := s.store.InsertProfile(profile); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, Conflict(fmt.Sprintf("Profile uniqueId already in use: %s", deref(profile.UniqueID)))
		}
		return nil, Internal(err)
	}
	return publicProfile(profile), nil
}

func (s *Profiles) Update(id string, value protocol.Value) (protocol.Value, error) {
	existing, err := s.store.Profile(id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, NotFound("Profile not found")
	}
	if err != nil {
		return nil, Internal(err)
	}
	fields := protocol.MapValue(value)
	profile := existing
	if err := setString(fields, "name", &profile.Name); err != nil {
		return nil, err
	}
	if profile.Name == "" {
		return nil, Invalid("name must be a non-empty string")
	}
	if raw, present := fields["connectType"]; present {
		v, valid := protocol.StringValue(raw)
		if !valid || (v != "ssh" && v != "command") {
			return nil, Invalid("connectType must be ssh or command")
		}
		profile.ConnectType = v
	}
	if err := setOptionalString(fields, "command", &profile.Command); err != nil {
		return nil, err
	}
	if err := setString(fields, "host", &profile.Host); err != nil {
		return nil, err
	}
	if err := setString(fields, "username", &profile.Username); err != nil {
		return nil, err
	}
	if err := setString(fields, "authType", &profile.AuthType); err != nil {
		return nil, err
	}
	if profile.AuthType != "password" && profile.AuthType != "key" {
		return nil, Invalid("authType must be password or key")
	}
	if err := setString(fields, "term", &profile.Term); err != nil {
		return nil, err
	}
	if err := setOptionalString(fields, "promptRegex", &profile.PromptRegex); err != nil {
		return nil, err
	}
	if err := setInteger(fields, "port", &profile.Port, 1, 65535); err != nil {
		return nil, err
	}
	if err := setInteger(fields, "cols", &profile.Cols, 20, 500); err != nil {
		return nil, err
	}
	if err := setInteger(fields, "rows", &profile.Rows, 5, 200); err != nil {
		return nil, err
	}
	if _, present := fields["uniqueId"]; present {
		if err := setOptionalString(fields, "uniqueId", &profile.UniqueID); err != nil {
			return nil, err
		}
		if profile.UniqueID != nil {
			trimmed := strings.TrimSpace(*profile.UniqueID)
			if trimmed == "" {
				profile.UniqueID = nil
			} else {
				profile.UniqueID = ptr(trimmed)
			}
		}
	}
	if err := validateTarget(profile); err != nil {
		return nil, err
	}
	if _, present := fields["uniqueId"]; present {
		if err := s.assertUniqueID(profile.UniqueID, id); err != nil {
			return nil, err
		}
	}
	for inputName, target := range map[string]**string{
		"password": &profile.EncryptedPassword, "privateKey": &profile.EncryptedPrivateKey, "passphrase": &profile.EncryptedPassphrase,
	} {
		if raw, present := fields[inputName]; present {
			if raw == nil {
				*target = nil
				continue
			}
			secret, valid := protocol.StringValue(raw)
			if !valid {
				return nil, Invalid(inputName + " must be a string")
			}
			if secret == "" {
				*target = nil
			} else {
				encrypted, encryptErr := s.box.Encrypt(secret)
				if encryptErr != nil {
					return nil, Internal(encryptErr)
				}
				*target = ptr(encrypted)
			}
		}
	}
	profile.UpdatedAt = time.Now().UnixMilli()
	if err := s.store.UpdateProfile(profile); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, NotFound("Profile not found")
		}
		return nil, Internal(err)
	}
	return publicProfile(profile), nil
}

func (s *Profiles) Delete(id string) error {
	if _, err := s.store.Profile(id); errors.Is(err, store.ErrNotFound) {
		return nil
	} else if err != nil {
		return Internal(err)
	}
	if err := s.store.DeleteProfile(id); err != nil {
		return Internal(err)
	}
	return nil
}

func (s *Profiles) assertUniqueID(value *string, excludeID string) error {
	if value == nil {
		return nil
	}
	profiles, err := s.store.Profiles("")
	if err != nil {
		return Internal(err)
	}
	for _, profile := range profiles {
		if profile.ID != excludeID && profile.UniqueID != nil && *profile.UniqueID == *value {
			return Conflict(fmt.Sprintf("Profile uniqueId already in use: %s", *value))
		}
	}
	return nil
}

func publicProfile(p store.Profile) protocol.Value {
	return protocol.Object(
		"id", p.ID, "name", p.Name, "uniqueId", p.UniqueID, "connectType", p.ConnectType,
		"command", p.Command, "host", p.Host, "port", p.Port, "username", p.Username,
		"authType", p.AuthType, "hasPassword", p.EncryptedPassword != nil,
		"hasPrivateKey", p.EncryptedPrivateKey != nil, "hasPassphrase", p.EncryptedPassphrase != nil,
		"term", p.Term, "cols", p.Cols, "rows", p.Rows, "promptRegex", p.PromptRegex,
		"createdAt", p.CreatedAt, "updatedAt", p.UpdatedAt,
	)
}

func validateTarget(p store.Profile) *AppError {
	if p.ConnectType == "command" && (p.Command == nil || strings.TrimSpace(*p.Command) == "") {
		return Invalid("Command profiles require a command")
	}
	if p.ConnectType == "ssh" && p.Host == "" {
		return Invalid("SSH profiles require a host")
	}
	if p.ConnectType == "ssh" && p.Username == "" {
		return Invalid("SSH profiles require a username")
	}
	return nil
}

func setString(fields map[string]protocol.Value, key string, target *string) *AppError {
	raw, present := fields[key]
	if !present {
		return nil
	}
	value, valid := protocol.StringValue(raw)
	if !valid {
		return Invalid(key + " must be a string")
	}
	*target = value
	return nil
}

func setOptionalString(fields map[string]protocol.Value, key string, target **string) *AppError {
	raw, present := fields[key]
	if !present {
		return nil
	}
	if raw == nil {
		*target = nil
		return nil
	}
	value, valid := protocol.StringValue(raw)
	if !valid {
		return Invalid(key + " must be a string or null")
	}
	*target = ptr(value)
	return nil
}

func optionalString(fields map[string]protocol.Value, key string) (string, bool) {
	value, present := fields[key]
	if !present {
		return "", true
	}
	text, valid := protocol.StringValue(value)
	return text, valid
}

func setInteger(fields map[string]protocol.Value, key string, target *int, min, max int) *AppError {
	raw, present := fields[key]
	if !present {
		return nil
	}
	value, valid := protocol.IntValue(raw)
	if !valid || value < int64(min) || value > int64(max) {
		return Invalid(fmt.Sprintf("%s must be an integer between %d and %d", key, min, max))
	}
	*target = int(value)
	return nil
}

func ptr[T any](value T) *T {
	return &value
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func randomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
