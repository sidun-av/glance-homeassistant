package render

import (
	"os/exec"
	"strings"
	"testing"
)

func houseWidgetData(lang string) WidgetData {
	d := sampleWidgetData()
	d.HouseURL = "/ha-widget/house/"
	d.Lang = lang
	return d
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
		`>План<`, `>3D<`, `Добавить в Dock`, `Не сейчас`, `Готово`, `Файл`,
		`data-house-url="/ha-widget/house/"`, `data-frame-title="Дом в 3D"`,
		`class="ha-house-invite" hidden`, `class="ha-house-frame"`, `haHouseInitialView`,
	} {
		if !strings.Contains(html, s) {
			t.Errorf("Russian house widget lacks %q", s)
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
		for _, s := range []string{`>Plan<`, `>3D<`, `Add to Dock`, `Not now`, `>Done<`} {
			if !strings.Contains(html, s) {
				t.Errorf("lang %q: lacks %q", lang, s)
			}
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
const safari17 = process.env.SAFARI17;
const sub = (from, to) => safari17.replace(from, to);
assert.equal(inv(safari17, 0, false, false, false), true, 'Safari 17 on a Mac');
assert.equal(inv(sub('Version/17.4', 'Version/18.0'), 0, false, false, false), true, 'Safari 18');
assert.equal(inv(sub('Version/17.4', 'Version/26.0'), 0, undefined, false, false), true, 'Safari 26, standalone undefined');
assert.equal(inv(safari17, 0, false, false, true), false, 'dismissed');
assert.equal(inv(safari17, 0, true, false, false), false, 'navigator.standalone');
assert.equal(inv(safari17, 0, false, true, false), false, 'display-mode standalone');
assert.equal(inv(safari17, 5, false, false, false), false, 'iPad (touch points)');
assert.equal(inv(safari17, undefined, false, false, false), false, 'no maxTouchPoints');
assert.equal(inv('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36', 0, false, false, false), false, 'Chrome');
assert.equal(inv('Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:131.0) Gecko/20100101 Firefox/131.0', 0, false, false, false), false, 'Firefox');
assert.equal(inv('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 Edg/129.0.0.0', 0, false, false, false), false, 'Edge');
assert.equal(inv(sub('Version/17.4', 'Version/16.6'), 0, false, false, false), false, 'Safari 16');
assert.equal(inv(sub('Version/17.4 ', ''), 0, false, false, false), false, 'no Version/');
assert.equal(inv('Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1', 0, false, false, false), false, 'iPhone');
assert.equal(inv('', 0, false, false, false), false, 'empty UA');
`
	cmd := exec.Command(node, "-e", houseDecideJS+assertions)
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
