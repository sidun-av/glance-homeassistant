package hass

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/websocket"
)

// maxWSMessage caps one websocket message (config/auth/list grows with the
// household).
const maxWSMessage = 4 << 20

// MintPersonKey returns a new long-lived token for the household user named
// person+" (Mac)", creating that user (local-only, system-users, not admin)
// on first use. HA only issues a long-lived token to the websocket session of
// the user it is for, so the user gets a throwaway password (never stored)
// and the token is minted by logging in as them.
func (c *Client) MintPersonKey(ctx context.Context, person, computer string) (string, error) {
	name := person + " (Mac)"
	sum := sha256.Sum256([]byte(person))
	username := "hc-" + hex.EncodeToString(sum[:])[:10]

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(raw)

	admin, err := c.wsDial(ctx, c.Token)
	if err != nil {
		return "", fmt.Errorf("admin session: %w", err)
	}
	defer admin.close()

	var users []struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		SystemGenerated bool   `json:"system_generated"`
	}
	res, err := admin.command(ctx, map[string]any{"type": "config/auth/list"})
	if err != nil {
		return "", fmt.Errorf("list users: %w", err)
	}
	if err := json.Unmarshal(res, &users); err != nil {
		return "", fmt.Errorf("parse users: %w", err)
	}
	userID := ""
	for _, u := range users {
		if !u.SystemGenerated && u.Name == name {
			userID = u.ID
			break
		}
	}

	if userID == "" {
		res, err := admin.command(ctx, map[string]any{
			"type":       "config/auth/create",
			"name":       name,
			"group_ids":  []string{"system-users"},
			"local_only": true,
		})
		if err != nil {
			return "", fmt.Errorf("create user: %w", err)
		}
		var created struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		}
		if err := json.Unmarshal(res, &created); err != nil || created.User.ID == "" {
			return "", fmt.Errorf("parse created user: no user id")
		}
		userID = created.User.ID
		if err := createCredentials(ctx, admin, userID, username, password); err != nil {
			return "", err
		}
	} else if _, err := admin.command(ctx, map[string]any{
		"type":     "config/auth_provider/homeassistant/admin_change_password",
		"user_id":  userID,
		"password": password,
	}); err != nil {
		// The user of an earlier run that stopped before its credentials
		// were made has no password to change: make them now.
		var werr *wsError
		if !errors.As(err, &werr) || werr.Code != "credentials_not_found" {
			return "", fmt.Errorf("change password: %w", err)
		}
		if err := createCredentials(ctx, admin, userID, username, password); err != nil {
			return "", err
		}
	}

	access, err := c.loginAs(ctx, username, password)
	if err != nil {
		return "", fmt.Errorf("login: %w", err)
	}
	sess, err := c.wsDial(ctx, access)
	if err != nil {
		return "", fmt.Errorf("user session: %w", err)
	}
	defer sess.close()
	clientName := "House Control — " + computer
	// HA allows one long-lived token per client name; a reinstall on the same
	// computer replaces (and so revokes) the previous key.
	res, err = sess.command(ctx, map[string]any{"type": "auth/refresh_tokens"})
	if err != nil {
		return "", fmt.Errorf("list tokens: %w", err)
	}
	var existing []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		ClientName string `json:"client_name"`
	}
	if err := json.Unmarshal(res, &existing); err != nil {
		return "", fmt.Errorf("parse tokens: %w", err)
	}
	for _, t := range existing {
		if t.Type == "long_lived_access_token" && t.ClientName == clientName {
			if _, err := sess.command(ctx, map[string]any{
				"type":             "auth/delete_refresh_token",
				"refresh_token_id": t.ID,
			}); err != nil {
				return "", fmt.Errorf("delete old token: %w", err)
			}
		}
	}
	res, err = sess.command(ctx, map[string]any{
		"type":        "auth/long_lived_access_token",
		"client_name": clientName,
		"lifespan":    3650,
	})
	if err != nil {
		return "", fmt.Errorf("long-lived token: %w", err)
	}
	var token string
	if err := json.Unmarshal(res, &token); err != nil || token == "" {
		return "", fmt.Errorf("parse long-lived token: unexpected result")
	}
	return token, nil
}

// createCredentials gives the user a username and password to log in with.
func createCredentials(ctx context.Context, admin *wsSession, userID, username, password string) error {
	if _, err := admin.command(ctx, map[string]any{
		"type":     "config/auth_provider/homeassistant/create",
		"user_id":  userID,
		"username": username,
		"password": password,
	}); err != nil {
		return fmt.Errorf("create credentials: %w", err)
	}
	return nil
}

// loginAs runs HA's login flow over REST for a local user and returns a
// short-lived access token. client_id and redirect_uri are the HA base URL,
// as HA requires them to share a host.
func (c *Client) loginAs(ctx context.Context, username, password string) (string, error) {
	clientID := c.BaseURL + "/"

	var flow struct {
		FlowID string `json:"flow_id"`
	}
	if err := c.restPost(ctx, "/auth/login_flow", "application/json", mustJSON(map[string]any{
		"client_id":    clientID,
		"handler":      []any{"homeassistant", nil},
		"redirect_uri": clientID,
	}), &flow); err != nil {
		return "", fmt.Errorf("start flow: %w", err)
	}
	if flow.FlowID == "" {
		return "", fmt.Errorf("start flow: no flow id")
	}

	var entry struct {
		Type   string `json:"type"`
		Result string `json:"result"`
	}
	if err := c.restPost(ctx, "/auth/login_flow/"+url.PathEscape(flow.FlowID), "application/json", mustJSON(map[string]any{
		"client_id": clientID,
		"username":  username,
		"password":  password,
	}), &entry); err != nil {
		return "", fmt.Errorf("submit credentials: %w", err)
	}
	if entry.Type != "create_entry" || entry.Result == "" {
		return "", fmt.Errorf("submit credentials: login refused (%s)", entry.Type)
	}

	form := url.Values{"grant_type": {"authorization_code"}, "code": {entry.Result}, "client_id": {clientID}}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.restPost(ctx, "/auth/token", "application/x-www-form-urlencoded", []byte(form.Encode()), &tok); err != nil {
		return "", fmt.Errorf("exchange code: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("exchange code: no access token")
	}
	return tok.AccessToken, nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// restPost sends an unauthenticated POST (the login endpoints take none) and
// decodes the JSON answer. Errors carry the status only, never the body,
// which may echo credentials.
func (c *Client) restPost(ctx context.Context, path, contentType string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxWSMessage)).Decode(out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

// wsSession is one authenticated HA websocket connection.
type wsSession struct {
	conn   *websocket.Conn
	nextID int
}

// wsDial opens /api/websocket and authenticates with token.
func (c *Client) wsDial(ctx context.Context, token string) (*wsSession, error) {
	u := c.BaseURL
	switch {
	case strings.HasPrefix(u, "https://"):
		u = "wss://" + strings.TrimPrefix(u, "https://")
	case strings.HasPrefix(u, "http://"):
		u = "ws://" + strings.TrimPrefix(u, "http://")
	}
	conn, _, err := websocket.Dial(ctx, u+"/api/websocket", nil)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	conn.SetReadLimit(maxWSMessage)
	s := &wsSession{conn: conn}

	var msg struct {
		Type string `json:"type"`
	}
	if err := s.read(ctx, &msg); err != nil {
		s.close()
		return nil, err
	}
	if msg.Type != "auth_required" {
		s.close()
		return nil, fmt.Errorf("unexpected greeting %q", msg.Type)
	}
	if err := s.write(ctx, map[string]any{"type": "auth", "access_token": token}); err != nil {
		s.close()
		return nil, err
	}
	if err := s.read(ctx, &msg); err != nil {
		s.close()
		return nil, err
	}
	if msg.Type != "auth_ok" {
		s.close()
		return nil, fmt.Errorf("authentication refused (%s)", msg.Type)
	}
	return s, nil
}

func (s *wsSession) close() { s.conn.Close(websocket.StatusNormalClosure, "") }

func (s *wsSession) write(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if err := s.conn.Write(ctx, websocket.MessageText, b); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

func (s *wsSession) read(ctx context.Context, v any) error {
	_, b, err := s.conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("receive: %w", err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("parse message: %w", err)
	}
	return nil
}

// wsError is a command's failed result: HA's error code and message.
type wsError struct{ Code, Message string }

func (e *wsError) Error() string { return e.Code + ": " + e.Message }

// command sends one id-numbered command and returns its result, skipping any
// unrelated messages. A failed result becomes an error naming HA's code and
// message.
func (s *wsSession) command(ctx context.Context, cmd map[string]any) (json.RawMessage, error) {
	s.nextID++
	id := s.nextID
	cmd["id"] = id
	if err := s.write(ctx, cmd); err != nil {
		return nil, err
	}
	for {
		var r struct {
			ID      int             `json:"id"`
			Type    string          `json:"type"`
			Success bool            `json:"success"`
			Result  json.RawMessage `json:"result"`
			Error   struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := s.read(ctx, &r); err != nil {
			return nil, err
		}
		if r.Type != "result" || r.ID != id {
			continue
		}
		if !r.Success {
			return nil, &wsError{Code: r.Error.Code, Message: r.Error.Message}
		}
		return r.Result, nil
	}
}
