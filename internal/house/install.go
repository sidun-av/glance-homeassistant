package house

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base32"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"
)

// PersonKeys mints a household member's own Home Assistant key; *hass.Client
// has it.
type PersonKeys interface {
	MintPersonKey(ctx context.Context, person, computer string) (string, error)
}

// The install routes, under /house/install/:
//
//	POST code         a one-time code and the path of its script
//	GET  <code>       the install script for that code (does not use it up);
//	                  for a used, expired or unknown code, a script that
//	                  says so in Russian and exits 1
//	POST <code>/key   {"person","computer"} → the person's HA key; uses the code up
//
// All of them are 404 until the server knows the app's URL and hash, can
// mint keys and has a plan to install.

const (
	installCodeTTL = 30 * time.Minute
	// installCodeRate and installCodeBurst bound new codes for the whole
	// process: 10 a minute.
	installCodeRate  = 10.0 / 60 // per second
	installCodeBurst = 10
	keyBudget        = 30 * time.Second
	maxKeyBytes      = 4 << 10
	maxNameRunes     = 80
)

var (
	//go:embed install.sh.tmpl
	installScriptSource string
	installScript       = template.Must(template.New("install.sh").
				Funcs(template.FuncMap{"sh": shellQuote}).
				Parse(installScriptSource))

	installCodeRe = regexp.MustCompile(`^[a-z2-7]{26}$`)
	// hostRe is a Host header that is safe to put into the script: a DNS
	// name or IPv4 address, or a bracketed IPv6 address, with an optional
	// port.
	hostRe = regexp.MustCompile(`^([A-Za-z0-9.-]+|\[[0-9A-Fa-f:.]+\])(:[0-9]{1,5})?$`)

	codeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)
)

// shellQuote is s as one single-quoted sh word. A value with a quote in it
// is refused rather than escaped: none of the script's values can carry one.
func shellQuote(s string) (string, error) {
	if strings.ContainsAny(s, "'\x00\n\r") {
		return "", errors.New("value cannot be shell-quoted")
	}
	return "'" + s + "'", nil
}

func (h *Handler) installEnabled() bool {
	return h.AppURL != "" && h.AppSHA256 != "" && h.Persons != nil && h.Store.Current() != nil
}

// install routes /house/install/*; root is <prefix>/house/install.
func (h *Handler) install(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !h.installEnabled() {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		if !h.sameSite(r) {
			log.Printf("house: install refused: Host %q or Origin %q is not this server", r.Host, r.Header.Get("Origin"))
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		rest, _ := strings.CutPrefix(r.URL.Path, root+"/")
		code, action, _ := strings.Cut(rest, "/")
		switch {
		case rest == "code":
			if !allowMethod(w, r, http.MethodPost) {
				return
			}
			h.newInstallCode(w)
		case !installCodeRe.MatchString(code):
			writeError(w, http.StatusNotFound, "not_found")
		case action == "" && !strings.HasSuffix(rest, "/"):
			if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
				return
			}
			h.installScriptFor(w, r, code)
		case action == "key":
			if !allowMethod(w, r, http.MethodPost) {
				return
			}
			h.installKey(w, r, code)
		default:
			writeError(w, http.StatusNotFound, "not_found")
		}
	})
}

// sameSite guards the install routes against DNS rebinding: a page on a
// site that a LAN visitor opens, its name re-pointed at this server, would
// otherwise be same-origin with it and could mint a key from the visitor's
// browser. Such a request names the foreign site in Host (and in Origin),
// so both must be this server: an IP literal, or one of InstallHosts.
// curl sends no Origin; the widget's own fetch does.
func (h *Handler) sameSite(r *http.Request) bool {
	if !h.ownHost(r.Host) {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Path == "" && h.ownHost(u.Host)
}

// ownHost is true for a Host (with or without a port) that is an IP
// literal or, case-insensitively, one of InstallHosts.
func (h *Handler) ownHost(hostport string) bool {
	if !hostRe.MatchString(hostport) {
		return false
	}
	host := hostport
	if strings.HasPrefix(host, "[") {
		host = host[1:strings.Index(host, "]")]
	} else if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	for _, allowed := range h.InstallHosts {
		if strings.EqualFold(host, allowed) {
			return true
		}
	}
	return false
}

func allowMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range methods {
		if r.Method == m {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	return false
}

// newInstallCode answers {"code","path"}: the widget turns path into
// `curl -fsSL '<origin><path>' | sh` (path is absolute when Base is).
func (h *Handler) newInstallCode(w http.ResponseWriter) {
	now := h.now()
	if !h.installRate.allow(now, installCodeRate, installCodeBurst) {
		w.Header().Set("Retry-After", "6")
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		log.Printf("house: install code: %v", err)
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	code := strings.ToLower(codeEncoding.EncodeToString(raw))
	h.installMu.Lock()
	for c, expires := range h.installCodes {
		if !now.Before(expires) {
			delete(h.installCodes, c)
		}
	}
	if h.installCodes == nil {
		h.installCodes = map[string]time.Time{}
	}
	h.installCodes[code] = now.Add(installCodeTTL)
	h.installMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"code": code, "path": h.Base + "/install/" + code})
}

// codeLive is true for a code issued and neither used nor expired.
func (h *Handler) codeLive(code string) bool {
	h.installMu.Lock()
	defer h.installMu.Unlock()
	expires, ok := h.installCodes[code]
	return ok && h.now().Before(expires)
}

// takeCode removes a live code, so that one request at a time can mint
// with it; giveBack returns it after a failed mint.
func (h *Handler) takeCode(code string) (expires time.Time, ok bool) {
	h.installMu.Lock()
	defer h.installMu.Unlock()
	expires, ok = h.installCodes[code]
	delete(h.installCodes, code)
	return expires, ok && h.now().Before(expires)
}

func (h *Handler) giveBack(code string, expires time.Time) {
	h.installMu.Lock()
	defer h.installMu.Unlock()
	h.installCodes[code] = expires
}

type installVars struct {
	Base, Code, AppURL, AppSHA256 string
}

// expiredScript is what a used, expired or unknown code gets instead of
// the install script: `curl -f` would print curl's own English error and
// hand sh nothing, so the answer is a 200 script that says in Russian
// where to get a new command. It carries no code and no app URL.
var expiredScript = template.Must(template.New("expired.sh").
	Funcs(template.FuncMap{"sh": shellQuote}).
	Parse(`#!/bin/sh
# House Control installer: this install code is used up or expired.
printf '%s\n' {{sh .}} >&2
exit 1
`))

func (h *Handler) installScriptFor(w http.ResponseWriter, r *http.Request, code string) {
	base, ok := absoluteBase(h.Base, r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_host")
		return
	}
	var out bytes.Buffer
	var err error
	if h.codeLive(code) {
		err = installScript.Execute(&out, installVars{Base: base, Code: code, AppURL: h.AppURL, AppSHA256: h.AppSHA256})
	} else {
		err = expiredScript.Execute(&out, "Ссылка устарела — откройте "+siteOf(base)+" и скопируйте команду заново.")
	}
	if err != nil {
		log.Printf("house: install script: %v", err)
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		w.Write(out.Bytes())
	}
}

// absoluteBase is the URL the script reaches this handler by. A Base that
// is already absolute (an origin public_url) is used as it is; a path Base
// gets the origin the request came to: the proxy's X-Forwarded-Proto (or
// the connection's own scheme) and the Host header, which must look like a
// host.
func absoluteBase(base string, r *http.Request) (string, bool) {
	if u, err := url.Parse(base); err == nil && u.Scheme != "" {
		return base, true
	}
	if !hostRe.MatchString(r.Host) {
		return "", false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := strings.ToLower(r.Header.Get("X-Forwarded-Proto")); p == "http" || p == "https" {
		scheme = p
	}
	return scheme + "://" + r.Host + base, true
}

// siteOf is the host (with any port) of an absolute base URL: what the
// install script calls SITE.
func siteOf(base string) string {
	_, rest, _ := strings.Cut(base, "://")
	site, _, _ := strings.Cut(rest, "/")
	return site
}

type keyRequest struct {
	Person   string `json:"person"`
	Computer string `json:"computer"`
}

// installKey mints the person's key. It is only for a computer on the home
// network: the TCP peer must be private and, when the request came through
// the reverse proxy (any forwarding header), the client address the proxy
// reports in X-Real-IP must be private too. A refused or invalid request
// leaves the code as it was.
func (h *Handler) installKey(w http.ResponseWriter, r *http.Request, code string) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return
	}
	if !fromHomeNetwork(r) {
		log.Printf("house: install key refused: not from the home network (peer %s)", r.RemoteAddr)
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxKeyBytes+1))
	if err != nil || len(raw) > maxKeyBytes {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	var req keyRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	person, okP := cleanName(req.Person)
	computer, okC := cleanName(req.Computer)
	if !okP || !okC {
		writeError(w, http.StatusBadRequest, "bad_name")
		return
	}
	expires, ok := h.takeCode(code)
	if !ok {
		writeError(w, http.StatusGone, "expired")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), keyBudget)
	defer cancel()
	token, err := h.Persons.MintPersonKey(ctx, person, computer)
	if err != nil {
		h.giveBack(code, expires)
		log.Printf("house: install key: %v", err)
		writeError(w, http.StatusBadGateway, "unavailable")
		return
	}
	log.Printf("house: install key minted for a household Mac")
	writeJSON(w, http.StatusOK, map[string]any{
		"homeassistant": map[string]string{"url": h.HAURL, "token": token, "name": "Home Assistant"},
	})
}

func fromHomeNetwork(r *http.Request) bool {
	if !privatePeer(r.RemoteAddr) {
		return false
	}
	proxied := false
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP", "Forwarded"} {
		proxied = proxied || r.Header.Get(header) != ""
	}
	if !proxied {
		return true
	}
	client, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP")))
	if err != nil {
		return false
	}
	client = client.Unmap()
	return client.IsLoopback() || client.IsPrivate()
}

// cleanName is a person or computer name trimmed, 1–80 runes of valid
// UTF-8 without control characters.
func cleanName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n == 0 || n > maxNameRunes || !utf8.ValidString(s) {
		return "", false
	}
	for _, c := range s {
		if unicode.IsControl(c) {
			return "", false
		}
	}
	return s, true
}
