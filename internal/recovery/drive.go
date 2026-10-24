package recovery

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

// Dedicated plaintext Drive remotes only. Never export unrelated accounts or
// arbitrary token/auth endpoints, credential commands or service-account keys.
func normalizeDriveConfig(data []byte, repository string) ([]byte, error) {
	if len(data) > 1024*1024 || !strings.HasPrefix(repository, "rclone:") {
		return nil, errors.New("kit transport must match a dedicated Drive remote")
	}
	parts := strings.SplitN(strings.TrimPrefix(repository, "rclone:"), ":", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid recovery remote")
	}
	remote := parts[0]
	values := map[string]string{}
	section := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if section || line != "["+remote+"]" {
				return nil, errors.New("kit requires one dedicated matching Drive remote")
			}
			section = true
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || !section {
			return nil, errors.New("invalid kit Drive config")
		}
		if _, exists := values[key]; exists {
			return nil, errors.New("duplicate kit Drive setting")
		}
		switch key {
		case "type", "client_id", "client_secret", "scope", "token", "use_trash", "root_folder_id", "team_drive":
		default:
			return nil, errors.New("unsupported Drive setting; kit export requires a reviewed dedicated config")
		}
		values[key] = value
	}
	if values["type"] != "drive" || values["scope"] != "drive.file" || !regexp.MustCompile(`^[0-9]+-[A-Za-z0-9_-]+\.apps\.googleusercontent\.com$`).MatchString(values["client_id"]) || !regexp.MustCompile(`^[A-Za-z0-9_-]{8,256}$`).MatchString(values["client_secret"]) {
		return nil, errors.New("kit supports dedicated Desktop Drive file-scope credentials only")
	}
	var token struct {
		Access  string    `json:"access_token"`
		Refresh string    `json:"refresh_token"`
		Type    string    `json:"token_type"`
		Expiry  time.Time `json:"expiry"`
	}
	if json.Unmarshal([]byte(values["token"]), &token) != nil || token.Access == "" || token.Refresh == "" || !strings.EqualFold(token.Type, "Bearer") || token.Expiry.IsZero() {
		return nil, errors.New("kit Drive token is absent or invalid")
	}
	normalized, _ := json.Marshal(token)
	text := "[" + remote + "]\ntype = drive\nclient_id = " + values["client_id"] + "\nclient_secret = " + values["client_secret"] + "\nscope = drive.file\nuse_trash = true\ntoken = " + string(normalized) + "\n"
	for _, field := range []string{"root_folder_id", "team_drive"} {
		if value := values[field]; value != "" {
			if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`).MatchString(value) {
				return nil, errors.New("invalid Drive folder/team identifier")
			}
			text += field + " = " + value + "\n"
		}
	}
	return []byte(text), nil
}
