// Package drive provides local-only desktop OAuth onboarding. Normal token
// refresh and Drive transport remain delegated to rclone.
package drive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"homevault/internal/config"
)

const Scope = "https://www.googleapis.com/auth/drive.file"
const authURL = "https://accounts.google.com/o/oauth2/v2/auth"
const tokenURL = "https://oauth2.googleapis.com/token"

var remoteName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)
var clientID = regexp.MustCompile(`^[0-9]+-[A-Za-z0-9_-]+\.apps\.googleusercontent\.com$`)
var clientSecret = regexp.MustCompile(`^[A-Za-z0-9_-]{8,256}$`)
var projectID = regexp.MustCompile(`^[a-z][a-z0-9-]{4,62}$`)

type metadata struct {
	Version int    `json:"version"`
	Remote  string `json:"remote"`
	Project string `json:"project"`
}

type wireToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

type connection struct {
	meta       metadata
	id         string
	secret     string
	token      *wireToken
	configHash string
}

func privateFile(file string, limit int64) ([]byte, error) {
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit || os.PathSeparator != '\\' && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("credential input must be an existing private regular file within size limit")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.New("cannot read private credential file")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("cannot read bounded credential file")
	}
	return data, nil
}

func configText(c connection) ([]byte, error) {
	text := "[" + c.meta.Remote + "]\ntype = drive\nclient_id = " + c.id + "\nclient_secret = " + c.secret + "\nscope = drive.file\nuse_trash = true\n"
	if c.token != nil {
		data, err := json.Marshal(c.token)
		if err != nil {
			return nil, errors.New("cannot encode OAuth token")
		}
		text += "token = " + string(data) + "\n"
	}
	return []byte(text), nil
}

func writeNew(file string, data []byte) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("private output must be new; existing credentials are never overwritten")
	}
	_, a := f.Write(data)
	b := f.Sync()
	c := f.Close()
	if a != nil || b != nil || c != nil {
		return errors.New("cannot persist private output; inspect partial setup")
	}
	return nil
}

// Setup reads only the downloaded Desktop client and creates a brand-new private
// directory. It performs no network request or authorization.
func Setup(root, clientFile, remote, expectedProject string) error {
	if err := config.ValidateDestination(root); err != nil {
		return err
	}
	if !filepath.IsAbs(clientFile) || !remoteName.MatchString(remote) {
		return errors.New("client JSON must be absolute and remote name must be safe")
	}
	data, err := privateFile(clientFile, 1024*1024)
	if err != nil {
		return err
	}
	var document struct {
		Installed struct {
			ID      string `json:"client_id"`
			Secret  string `json:"client_secret"`
			Project string `json:"project_id"`
			Auth    string `json:"auth_uri"`
			Token   string `json:"token_uri"`
		} `json:"installed"`
	}
	if json.Unmarshal(data, &document) != nil {
		return errors.New("invalid Desktop OAuth client JSON")
	}
	v := document.Installed
	if !clientID.MatchString(v.ID) || !clientSecret.MatchString(v.Secret) || !projectID.MatchString(v.Project) || v.Auth != "" && v.Auth != authURL && v.Auth != "https://accounts.google.com/o/oauth2/auth" || v.Token != "" && v.Token != tokenURL {
		return errors.New("expected a Google Desktop OAuth client with official endpoints")
	}
	if expectedProject != "" && v.Project != expectedProject {
		return errors.New("OAuth project does not match expected project")
	}
	if err := os.Mkdir(root, 0700); err != nil {
		return errors.New("OAuth directory must be new and its parent must exist")
	}
	c := connection{meta: metadata{Version: 1, Remote: remote, Project: v.Project}, id: v.ID, secret: v.Secret}
	contents, _ := configText(c)
	if err := writeNew(filepath.Join(root, "rclone.conf"), contents); err != nil {
		return err
	}
	meta, _ := json.MarshalIndent(c.meta, "", "  ")
	return writeNew(filepath.Join(root, "connection.json"), meta)
}

func readConnection(root string) (connection, error) {
	var c connection
	if config.ValidateDestination(root) != nil {
		return c, errors.New("OAuth directory must be absolute")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || os.PathSeparator != '\\' && info.Mode().Perm()&0077 != 0 {
		return c, errors.New("OAuth directory must be private and not a symlink")
	}
	data, err := privateFile(filepath.Join(root, "connection.json"), 1024*1024)
	if err != nil {
		return c, err
	}
	if json.Unmarshal(data, &c.meta) != nil || c.meta.Version != 1 || !remoteName.MatchString(c.meta.Remote) || !projectID.MatchString(c.meta.Project) {
		return c, errors.New("invalid OAuth connection metadata")
	}
	data, err = privateFile(filepath.Join(root, "rclone.conf"), 1024*1024)
	if err != nil {
		return c, err
	}
	hash := sha256.Sum256(data)
	c.configHash = hex.EncodeToString(hash[:])
	values := map[string]string{}
	section := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if section || line != "["+c.meta.Remote+"]" {
				return c, errors.New("OAuth config must contain only the prepared Drive remote")
			}
			section = true
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !section || !ok {
			return c, errors.New("invalid OAuth configuration")
		}
		if _, exists := values[key]; exists {
			return c, errors.New("duplicate OAuth config setting")
		}
		switch key {
		case "type", "client_id", "client_secret", "scope", "use_trash", "token":
		default:
			return c, errors.New("OAuth config has unsupported settings; review changes privately")
		}
		values[key] = value
	}
	if values["type"] != "drive" || values["scope"] != "drive.file" || values["use_trash"] != "true" || !clientID.MatchString(values["client_id"]) || !clientSecret.MatchString(values["client_secret"]) {
		return c, errors.New("OAuth Drive type/client/scope/trash policy changed")
	}
	c.id, c.secret = values["client_id"], values["client_secret"]
	if raw := values["token"]; raw != "" {
		var token wireToken
		if json.Unmarshal([]byte(raw), &token) != nil || token.AccessToken == "" || token.RefreshToken == "" || !strings.EqualFold(token.TokenType, "Bearer") || token.Expiry.IsZero() {
			return c, errors.New("invalid saved OAuth token")
		}
		c.token = &token
	}
	return c, nil
}
