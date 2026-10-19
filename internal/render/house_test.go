package render

import (
	"os/exec"
	"strings"
	"testing"
)

func houseWidgetData(lang string) WidgetData {
	d := sampleWidgetData()
	d.HouseURL = "/ha-widget/house/"
	d.HouseInstall = true
	d.Lang = lang
	return d
}

// houseInvite is the invite card's markup in a rendered widget.
func houseInvite(html string) string {
	i, j := strings.Index(html, `class="ha-house-invite"`), strings.Index(html, `class="ha-house-frame"`)
	if i < 0 || j < i {
		return ""
	}
	return html[i:j]
}

func TestRenderWidget_NoHouseURLNoSwitch(t *testing.T) {
	html := RenderWidget(sampleWidgetData())
	for _, s := range []string{"ha-house-switch", "ha-house-frame", "ha-house-invite", "haHouseInitialView", "<iframe"} {
		if strings.Contains(html, s) {
			t.Errorf("widget without a house URL contains %q", s)
		}
	}
}

func TestRenderWidget_HouseSwitchRussian(t *testing.T) {
	html := RenderWidget(houseWidgetData("ru"))
	for _, s := range []string{
		`>План<`, `>3D<`, `data-house-url="/ha-widget/house/"`, `data-frame-title="Дом в 3D"`,
		`class="ha-house-invite" hidden`, `class="ha-house-frame"`, `haHouseInitialView`,
		`<b>House Control на Mac</b>`, `3D-дом в строке меню и в «чёлке», уже настроенный на ваш дом.`,
		`data-house-act="install">Установить<`, `data-house-act="later">Напомнить завтра<`,
		`data-house-act="close" aria-label="Не показывать"`,
		`1. Откройте Терминал (⌘ Пробел → «Терминал»).`, `2. Нажмите «Скопировать команду».`,
		`3. Вставьте её в Терминал (⌘V) и нажмите Enter.`,
		`data-house-act="copy">Скопировать команду<`, `data-house-act="done">Готово<`,
		`data-copied="Скопировано ✓"`, `data-failed="Не получилось — обновите страницу"`,
		`Скопируйте: ⌘C`, `data-code-url="/ha-widget/house/install/code"`, `>Терминал</text>`,
	} {
		if !strings.Contains(html, s) {
			t.Errorf("Russian house widget lacks %q", s)
		}
	}
	// The vacuum popover has a Dock button of its own; the invite and the
	// script must not mention the Dock or Safari any more.
	for _, s := range []string{"Dock", "Safari", "data-house-dock"} {
		if strings.Contains(houseInvite(html), s) || strings.Contains(houseScript, s) {
			t.Errorf("Russian house widget still has %q", s)
		}
	}
	if strings.Contains(html, "<iframe") {
		t.Errorf("the server HTML carries an iframe; it must be created lazily by the script")
	}
	// The switch sits in the section head, before the live badge.
	head := html[strings.Index(html, `<div class="ha-section-head">`):]
	head = head[:strings.Index(head, `</span></div>`)]
	if sw, badge := strings.Index(head, "ha-house-switch"), strings.Index(head, "ha-live-badge"); sw < 0 || badge < 0 || sw > badge {
		t.Errorf("switch not before the live badge in the head: %s", head)
	}
	// Toggle buttons, not a half-built ARIA tab pattern.
	if !strings.Contains(head, `role="group"`) || strings.Count(head, `aria-pressed="false"`) != 2 || strings.Contains(head, `role="tab`) {
		t.Errorf("switch is not a group of aria-pressed buttons: %s", head)
	}
	// The invite and the frame are siblings of the map, inside the widget.
	if i, m := strings.Index(html, "ha-house-invite"), strings.Index(html, `class="ha-rooms"`); i < 0 || m < 0 || i > m {
		t.Errorf("invite is not above the cards")
	}
}

func TestRenderWidget_HouseCopyEnglishDefault(t *testing.T) {
	for _, lang := range []string{"", "en", "xx"} {
		html := RenderWidget(houseWidgetData(lang))
		for _, s := range []string{`>Plan<`, `>3D<`, `>Install<`, `>Remind me tomorrow<`, `aria-label="Don&#39;t show again"`,
			`>Copy command<`, `>Done<`, `>Terminal</text>`} {
			if !strings.Contains(html, s) {
				t.Errorf("lang %q: lacks %q", lang, s)
			}
		}
		if strings.Contains(houseInvite(html), "Dock") {
			t.Errorf("lang %q: Dock copy left in", lang)
		}
		if strings.Contains(html, "План") {
			t.Errorf("lang %q: Russian copy in the English widget", lang)
		}
	}
}

func TestRenderWidget_HouseURLIsEscaped(t *testing.T) {
	d := houseWidgetData("en")
	d.HouseURL = `/x/"><script>/house/`
	html := RenderWidget(d)
	if strings.Contains(html, `"><script>`) {
		t.Errorf("house URL not escaped")
	}
}

// Without installs configured the switch and the 3D view stay, the invite
// does not.
func TestRenderWidget_NoInviteWithoutInstalls(t *testing.T) {
	d := houseWidgetData("ru")
	d.HouseInstall = false
	html := RenderWidget(d)
	if strings.Contains(html, `class="ha-house-invite"`) {
		t.Errorf("invite rendered while installs are off")
	}
	for _, s := range []string{"ha-house-switch", "ha-house-frame", "houseBooted"} {
		if !strings.Contains(html, s) {
			t.Errorf("widget without installs lacks %q", s)
		}
	}
}

func TestHouseScript_Decisions(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	const safari17 = `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15`
	const assertions = `
const assert = require('node:assert/strict');
const iv = haHouseInitialView, inv = haHouseShouldInvite;
assert.equal(iv(null, false), '3d');
assert.equal(iv(null, true), 'plan');
assert.equal(iv('plan', false), 'plan');
assert.equal(iv('3d', true), '3d');
assert.equal(iv('junk', false), '3d');
assert.equal(iv('junk', true), 'plan');
assert.equal(iv(undefined, false), '3d');
const safari = process.env.SAFARI17;
const chrome = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36';
const firefox = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:131.0) Gecko/20100101 Firefox/131.0';
const iphone = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1';
const now = 1760000000000;
assert.equal(inv(safari, 0, null, null, now), true, 'Safari on a Mac');
assert.equal(inv(chrome, 0, null, null, now), true, 'Chrome on a Mac');
assert.equal(inv(firefox, 0, null, null, now), true, 'Firefox on a Mac');
assert.equal(inv(safari, 5, null, null, now), false, 'iPad (Macintosh UA, touch points)');
assert.equal(inv(safari, undefined, null, null, now), false, 'no maxTouchPoints');
assert.equal(inv(iphone, 0, null, null, now), false, 'iPhone');
assert.equal(inv('', 0, null, null, now), false, 'empty UA');
assert.equal(inv(chrome, 0, 'dismissed', null, now), false, 'dismissed');
assert.equal(inv(chrome, 0, 'started:' + (now - 1000), null, now), false, 'started a second ago');
assert.equal(inv(chrome, 0, 'started:' + (now - 864e5 + 1000), null, now), false, 'started almost a day ago');
assert.equal(inv(chrome, 0, 'started:' + (now - 864e5), null, now), true, 'started a day ago: a fresh command');
assert.equal(inv(chrome, 0, 'started:junk', null, now), true, 'started at a junk time');
assert.equal(inv(chrome, 0, 'started', null, now), true, 'started with no time');
assert.equal(inv(chrome, 0, 'junk', null, now), true, 'unknown state');
assert.equal(inv(chrome, 0, null, String(now + 1000), now), false, 'remind in the future');
assert.equal(inv(chrome, 0, null, String(now - 1000), now), true, 'remind in the past');
assert.equal(inv(chrome, 0, null, String(now), now), true, 'remind now');
assert.equal(inv(chrome, 0, null, 'junk', now), true, 'junk remind');
const done = haHouseDoneState;
assert.equal(done('started:' + now), 'dismissed', '«Готово» after a copy');
assert.equal(inv(chrome, 0, done('started:' + now), null, now + 2 * 864e5), false, 'a finished install never comes back');
assert.equal(done(null), null, '«Готово» without a copy stores nothing');
assert.equal(done('junk'), null, '«Готово» over an unknown state');
const cmd = haHouseCommand, fresh = haHouseCodeFresh;
assert.equal(cmd('/ha-widget/house/install/abc', 'http://dash.example:8080'), "curl -fsSL 'http://dash.example:8080/ha-widget/house/install/abc' | sh");
assert.equal(cmd('https://home.example/ha-widget/house/install/abc', 'http://10.0.0.5'), "curl -fsSL 'https://home.example/ha-widget/house/install/abc' | sh", 'absolute path');
assert.equal(fresh(0, now), false, 'no code yet');
assert.equal(fresh(now - 24 * 60e3, now), true, 'a 24-minute-old code');
assert.equal(fresh(now - 25 * 60e3, now), false, 'a 25-minute-old code');
`
	// An onerror handler runs with the document in scope, where URL is
	// document.URL, a string: the with block shadows it the same way.
	cmd := exec.Command(node, "-e", "with({URL: 'http://shadow.example/'}){"+houseDecideJS+assertions+"}")
	cmd.Env = append(cmd.Environ(), "SAFARI17="+safari17)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
}

func TestHouseScript_Guarded(t *testing.T) {
	if !strings.Contains(houseScript, "root.dataset.houseBooted)return;root.dataset.houseBooted='1'") {
		t.Errorf("houseScript has no houseBooted guard")
	}
	for _, s := range []string{"root.innerHTML", "replaceWith", "replaceChildren", "outerHTML"} {
		if strings.Contains(houseScript, s) {
			t.Errorf("houseScript uses %s", s)
		}
	}
	if !strings.Contains(houseScript, houseDecideJS) {
		t.Errorf("houseScript does not embed houseDecideJS")
	}
	// Every storage access is wrapped.
	for i := strings.Index(houseScript, "localStorage"); i >= 0; {
		if !strings.HasSuffix(houseScript[:i], "try{") && !strings.HasSuffix(houseScript[:i], "try{return ") {
			t.Errorf("localStorage outside a try at %q", houseScript[max(0, i-20):i+20])
		}
		next := strings.Index(houseScript[i+1:], "localStorage")
		if next < 0 {
			break
		}
		i += 1 + next
	}
}

// The script only runs when there is a house: it is not part of the
// payload otherwise, and when present it runs after the widget's own
// bootstrap in the same onerror.
func TestRenderWidget_HouseScriptOnlyWithHouse(t *testing.T) {
	if html := RenderWidget(sampleWidgetData()); strings.Contains(html, "houseBooted") {
		t.Errorf("house script present without a house URL")
	}
	if html := RenderWidget(houseWidgetData("ru")); !strings.Contains(html, "houseBooted") {
		t.Errorf("house script missing")
	}
}

// The onerror payload is several scripts concatenated; with the house
// script it must still run as one function body: each part returns early
// on an img outside a widget, and none calls the previous one's result.
func TestHouseScript_PayloadRuns(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	payload := bootstrapScript + entityControlScript + tempChartScript + houseScript
	cmd := exec.Command(node, "-e", "new Function(process.env.PAYLOAD).call({closest: () => null})")
	cmd.Env = append(cmd.Environ(), "PAYLOAD="+payload)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("payload does not run: %v\n%s", err, out)
	}
}
