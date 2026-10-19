package hass

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const (
	fakeAdminToken  = "admin-token-xyz"
	fakeAccessToken = "short-access-abc"
	fakeLongToken   = "long-lived-def"
)

// fakeHA is an httptest stand-in for HA's websocket API and login flow.
type fakeHA struct {
	srv *httptest.Server

	mu       sync.Mutex
	users    []map[string]any
	commands []map[string]any // every ws command received, with the session's token
	failType string           // ws command type to answer with an error

	loginUser, loginPass string
	llTokens             []map[string]any // the user's long-lived refresh tokens
	minted               int
}

func newFakeHA(t *testing.T, users ...map[string]any) *fakeHA {
	f := &fakeHA{users: users}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/websocket", f.ws)
	mux.HandleFunc("/auth/login_flow", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		if b["client_id"] != f.srv.URL+"/" || b["redirect_uri"] != f.srv.URL+"/" {
			http.Error(w, "bad client", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"flow_id": "flow1", "type": "form"})
	})
	mux.HandleFunc("/auth/login_flow/flow1", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		f.mu.Lock()
		ok := b["username"] == f.loginUser && b["password"] == f.loginPass
		f.mu.Unlock()
		if !ok {
			json.NewEncoder(w).Encode(map[string]any{"type": "form", "errors": map[string]any{"base": "invalid_auth"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "create_entry", "result": "code1"})
	})
	mux.HandleFunc("/auth/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "code1" {
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": fakeAccessToken})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeHA) ws(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := r.Context()
	send := func(v any) {
		b, _ := json.Marshal(v)
		conn.Write(ctx, websocket.MessageText, b)
	}
	send(map[string]any{"type": "auth_required"})
	_, b, err := conn.Read(ctx)
	if err != nil {
		return
	}
	var auth struct {
		Token string `json:"access_token"`
	}
	json.Unmarshal(b, &auth)
	if auth.Token != fakeAdminToken && auth.Token != fakeAccessToken {
		send(map[string]any{"type": "auth_invalid"})
		return
	}
	send(map[string]any{"type": "auth_ok"})
	for {
		_, b, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var cmd map[string]any
		json.Unmarshal(b, &cmd)
		typ, _ := cmd["type"].(string)
		id := cmd["id"]

		f.mu.Lock()
		cmd["_token"] = auth.Token
		f.commands = append(f.commands, cmd)
		fail := f.failType == typ
		f.mu.Unlock()

		if fail {
			send(map[string]any{"id": id, "type": "result", "success": false,
				"error": map[string]any{"code": "unknown_error", "message": "boom"}})
			continue
		}
		var result any
		switch typ {
		case "config/auth/list":
			f.mu.Lock()
			result = f.users
			f.mu.Unlock()
		case "config/auth/create":
			result = map[string]any{"user": map[string]any{"id": "new-id"}}
		case "config/auth_provider/homeassistant/create":
			f.mu.Lock()
			f.loginUser, f.loginPass = cmd["username"].(string), cmd["password"].(string)
			f.mu.Unlock()
		case "config/auth_provider/homeassistant/admin_change_password":
			f.mu.Lock()
			// A user without credentials (none were made for it) has no
			// password to change.
			if f.loginUser == "" {
				f.mu.Unlock()
				send(map[string]any{"id": id, "type": "result", "success": false,
					"error": map[string]any{"code": "credentials_not_found", "message": "Credentials not found"}})
				continue
			}
			f.loginPass = cmd["password"].(string)
			f.mu.Unlock()
		case "auth/refresh_tokens":
			f.mu.Lock()
			result = append([]map[string]any{{"id": "rt-web", "type": "normal", "client_name": nil}}, f.llTokens...)
			f.mu.Unlock()
		case "auth/delete_refresh_token":
			f.mu.Lock()
			for i, t := range f.llTokens {
				if t["id"] == cmd["refresh_token_id"] {
					f.llTokens = append(f.llTokens[:i], f.llTokens[i+1:]...)
					break
				}
			}
			f.mu.Unlock()
		case "auth/long_lived_access_token":
			f.mu.Lock()
			dup := false
			for _, t := range f.llTokens {
				dup = dup || t["client_name"] == cmd["client_name"]
			}
			if dup {
				f.mu.Unlock()
				send(map[string]any{"id": id, "type": "result", "success": false,
					"error": map[string]any{"code": "home_assistant_error", "message": "already exists"}})
				continue
			}
			f.minted++
			f.llTokens = append(f.llTokens, map[string]any{"id": "rt-" + string(rune('0'+f.minted)), "type": "long_lived_access_token", "client_name": cmd["client_name"]})
			f.mu.Unlock()
			result = fakeLongToken
		}
		send(map[string]any{"id": id, "type": "result", "success": true, "result": result})
	}
}

func (f *fakeHA) commandsOf(typ string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, c := range f.commands {
		if c["type"] == typ {
			out = append(out, c)
		}
	}
	return out
}

func mint(t *testing.T, f *fakeHA, person, computer string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New(f.srv.URL, fakeAdminToken).MintPersonKey(ctx, person, computer)
}

func TestMintPersonKey_CreatesUser(t *testing.T) {
	f := newFakeHA(t, map[string]any{"id": "sys", "name": "Demo (Mac)", "system_generated": true})
	// A system-generated user of the same name must be ignored.
	token, err := mint(t, f, "Demo", "demo-mac")
	if err != nil {
		t.Fatal(err)
	}
	if token != fakeLongToken {
		t.Fatalf("token = %q", token)
	}
	creates := f.commandsOf("config/auth/create")
	if len(creates) != 1 {
		t.Fatalf("create calls = %d, want 1", len(creates))
	}
	c := creates[0]
	if c["name"] != "Demo (Mac)" || c["local_only"] != true {
		t.Errorf("create payload = %v", c)
	}
	groups, _ := c["group_ids"].([]any)
	if len(groups) != 1 || groups[0] != "system-users" {
		t.Errorf("group_ids = %v, want [system-users]", c["group_ids"])
	}
	if _, has := c["is_admin"]; has {
		t.Errorf("create must not set is_admin: %v", c)
	}
	cred := f.commandsOf("config/auth_provider/homeassistant/create")
	if len(cred) != 1 || cred[0]["user_id"] != "new-id" {
		t.Fatalf("credentials = %v", cred)
	}
	if u, _ := cred[0]["username"].(string); !strings.HasPrefix(u, "hc-") || len(u) != 13 {
		t.Errorf("username = %q", u)
	}
	if p, _ := cred[0]["password"].(string); len(p) < 40 {
		t.Errorf("password too short: %d", len(p))
	}
}

func TestMintPersonKey_ReusesUser(t *testing.T) {
	f := newFakeHA(t, map[string]any{"id": "u1", "name": "Demo (Mac)"})
	sum := sha256.Sum256([]byte("Demo"))
	f.loginUser = "hc-" + hex.EncodeToString(sum[:])[:10]
	token, err := mint(t, f, "Demo", "demo-mac")
	if err != nil || token != fakeLongToken {
		t.Fatalf("token, err = %q, %v", token, err)
	}
	if n := len(f.commandsOf("config/auth/create")); n != 0 {
		t.Errorf("create calls = %d, want 0", n)
	}
	if n := len(f.commandsOf("config/auth_provider/homeassistant/create")); n != 0 {
		t.Errorf("credential creates = %d, want 0", n)
	}
	pw := f.commandsOf("config/auth_provider/homeassistant/admin_change_password")
	if len(pw) != 1 || pw[0]["user_id"] != "u1" {
		t.Fatalf("password changes = %v", pw)
	}
}

// An earlier run created the user but stopped before its credentials:
// they are made now instead of changing a password that is not there.
func TestMintPersonKey_UserWithoutCredentials(t *testing.T) {
	f := newFakeHA(t, map[string]any{"id": "u1", "name": "Demo (Mac)"})
	token, err := mint(t, f, "Demo", "demo-mac")
	if err != nil || token != fakeLongToken {
		t.Fatalf("token, err = %q, %v", token, err)
	}
	if n := len(f.commandsOf("config/auth/create")); n != 0 {
		t.Errorf("create calls = %d, want 0", n)
	}
	sum := sha256.Sum256([]byte("Demo"))
	cred := f.commandsOf("config/auth_provider/homeassistant/create")
	if len(cred) != 1 || cred[0]["user_id"] != "u1" || cred[0]["username"] != "hc-"+hex.EncodeToString(sum[:])[:10] {
		t.Fatalf("credentials = %v, want one for u1", cred)
	}

	// Any other failure to change the password is still an error.
	f = newFakeHA(t, map[string]any{"id": "u1", "name": "Demo (Mac)"})
	f.loginUser = "hc-" + hex.EncodeToString(sum[:])[:10]
	f.failType = "config/auth_provider/homeassistant/admin_change_password"
	if _, err := mint(t, f, "Demo", "demo-mac"); err == nil || !strings.Contains(err.Error(), "change password") {
		t.Errorf("err = %v, want it to name 'change password'", err)
	}
	if n := len(f.commandsOf("config/auth_provider/homeassistant/create")); n != 0 {
		t.Errorf("credential creates after another error = %d, want 0", n)
	}
}

func TestMintPersonKey_TokenName(t *testing.T) {
	f := newFakeHA(t)
	if _, err := mint(t, f, "Demo", "demo-mac"); err != nil {
		t.Fatal(err)
	}
	ll := f.commandsOf("auth/long_lived_access_token")
	if len(ll) != 1 {
		t.Fatalf("long-lived calls = %d", len(ll))
	}
	if ll[0]["client_name"] != "House Control — demo-mac" || ll[0]["lifespan"] != float64(3650) {
		t.Errorf("payload = %v", ll[0])
	}
	if ll[0]["_token"] != fakeAccessToken {
		t.Errorf("minted with %v, want the user's own access token", ll[0]["_token"])
	}
	for _, c := range f.commandsOf("config/auth/list") {
		if c["_token"] != fakeAdminToken {
			t.Errorf("admin commands must use the admin token")
		}
	}
}

func TestMintPersonKey_HAErrorNamesStep(t *testing.T) {
	f := newFakeHA(t)
	f.failType = "config/auth/create"
	_, err := mint(t, f, "Demo", "demo-mac")
	if err == nil || !strings.Contains(err.Error(), "create user") {
		t.Fatalf("err = %v, want it to name 'create user'", err)
	}
	if strings.Contains(err.Error(), fakeAdminToken) {
		t.Errorf("error leaks the admin token: %v", err)
	}

	f = newFakeHA(t)
	f.failType = "config/auth_provider/homeassistant/create"
	_, err = mint(t, f, "Demo", "demo-mac")
	if err == nil || !strings.Contains(err.Error(), "create credentials") {
		t.Fatalf("err = %v, want it to name 'create credentials'", err)
	}
	pw := f.commandsOf("config/auth_provider/homeassistant/create")[0]["password"].(string)
	if strings.Contains(err.Error(), pw) || strings.Contains(err.Error(), fakeAdminToken) {
		t.Errorf("error leaks a secret: %v", err)
	}

	f = newFakeHA(t)
	f.failType = "auth/long_lived_access_token"
	_, err = mint(t, f, "Demo", "demo-mac")
	if err == nil || !strings.Contains(err.Error(), "long-lived token") {
		t.Fatalf("err = %v, want it to name 'long-lived token'", err)
	}
	if strings.Contains(err.Error(), fakeAccessToken) {
		t.Errorf("error leaks the access token: %v", err)
	}
}

func TestMintPersonKey_SecondMintReplacesOldToken(t *testing.T) {
	f := newFakeHA(t)
	for i := 0; i < 2; i++ {
		if _, err := mint(t, f, "Demo", "demo-mac"); err != nil {
			t.Fatalf("mint %d: %v", i+1, err)
		}
	}
	del := f.commandsOf("auth/delete_refresh_token")
	if len(del) != 1 || del[0]["refresh_token_id"] != "rt-1" {
		t.Fatalf("deletes = %v, want exactly the first token (rt-1)", del)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.llTokens) != 1 || f.llTokens[0]["id"] != "rt-2" {
		t.Errorf("remaining long-lived tokens = %v, want only rt-2", f.llTokens)
	}
}
