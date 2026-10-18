package render

import (
	"fmt"
	"html"
	"strings"
)

// The «Plan | 3D» switch, the lazy iframe holding the House Control 3D
// page, and the Safari "Add to Dock" invite. None of it is rendered unless
// WidgetData.HouseURL is set (public_url configured and a plan uploaded),
// so a widget without a plan is byte-for-byte what it was before.
//
// Glance renders a page once: it fetches the page's content and assigns
// it with innerHTML when the page loads, and never re-renders widgets
// while the page stays open (an extension's cache only decides when the
// server fetches /widget again for the next page load). So an iframe
// created here lives until the page reloads, provided nothing in this
// widget's own scripts replaces or moves it: the live poll only edits
// data-* attributes inside the map, and the house script below only ever
// appends the iframe into .ha-house-frame and removes it when the viewer
// picks «Plan».

// houseCopy is the widget-side text of the 3D view, in HOUSE_LANG.
type houseCopy struct {
	Plan, ThreeD                                    string
	InviteTitle, InviteBody, InviteAdd, InviteLater string
	StepsTitle, StepsBody, StepsDone                string
	FrameTitle                                      string
	// The picture of Safari's menu bar and its File menu in the steps
	// card: MenuBar[0] is the open menu; MenuItems "" is a separator, and
	// the item equal to MenuAdd is drawn highlighted.
	MenuBar   []string
	MenuItems []string
	MenuAdd   string
}

func houseStrings(lang string) houseCopy {
	if lang == "ru" {
		return houseCopy{
			Plan:        "План",
			ThreeD:      "3D",
			InviteTitle: "Дом — как приложение",
			InviteBody:  "Добавьте дашборд в Dock: он откроется в своём окне, сразу с 3D-домом.",
			InviteAdd:   "Добавить в Dock",
			InviteLater: "Не сейчас",
			StepsTitle:  "Один шаг",
			StepsBody:   "В меню Safari выберите «Файл» → «Добавить в Dock…» и нажмите «Добавить».",
			StepsDone:   "Готово",
			FrameTitle:  "Дом в 3D",
			MenuBar:     []string{"Файл", "Правка", "Вид"},
			MenuItems:   []string{"Новое окно", "Новая вкладка", "Открыть файл…", "", "Добавить в Dock…", "", "Закрыть окно"},
			MenuAdd:     "Добавить в Dock…",
		}
	}
	return houseCopy{
		Plan:        "Plan",
		ThreeD:      "3D",
		InviteTitle: "Your home as an app",
		InviteBody:  "Add the dashboard to the Dock: it opens in its own window, straight into the 3D home.",
		InviteAdd:   "Add to Dock",
		InviteLater: "Not now",
		StepsTitle:  "One step",
		StepsBody:   "In Safari choose File → Add to Dock… and click Add.",
		StepsDone:   "Done",
		FrameTitle:  "Home in 3D",
		MenuBar:     []string{"File", "Edit", "View"},
		MenuItems:   []string{"New Window", "New Tab", "Open File…", "", "Add to Dock…", "", "Close Window"},
		MenuAdd:     "Add to Dock…",
	}
}

// renderHouseSwitch is the segmented control in .ha-section-head. Which
// segment is selected is decided in the browser (stored choice, screen
// width), so both start unselected.
func renderHouseSwitch(c houseCopy) string {
	return `<div class="ha-house-switch" role="group">` +
		`<button type="button" data-house-view="plan" aria-pressed="false">` + html.EscapeString(c.Plan) + `</button>` +
		`<button type="button" data-house-view="3d" aria-pressed="false">` + html.EscapeString(c.ThreeD) + `</button>` +
		`</div>`
}

// renderHouseParts is the invite card (hidden until the script decides
// this browser should see it) and the empty frame container the script
// creates the iframe in. Both are siblings of the map/cards.
func renderHouseParts(url string, c houseCopy) string {
	e := html.EscapeString
	var b strings.Builder
	b.WriteString(`<div class="ha-house-invite" hidden data-step="ask">`)
	b.WriteString(`<div class="ha-house-invite-ask">` + houseDockIcon +
		`<div class="ha-house-invite-text"><b>` + e(c.InviteTitle) + `</b><span>` + e(c.InviteBody) + `</span></div>` +
		`<div class="ha-house-invite-actions"><button type="button" class="ha-house-primary" data-house-dock="add">` + e(c.InviteAdd) +
		`</button><button type="button" data-house-dock="later">` + e(c.InviteLater) + `</button></div></div>`)
	b.WriteString(`<div class="ha-house-invite-steps">` + houseMenuPicture(c) +
		`<div class="ha-house-invite-text"><b>` + e(c.StepsTitle) + `</b><span>` + e(c.StepsBody) + `</span></div>` +
		`<div class="ha-house-invite-actions"><button type="button" class="ha-house-primary" data-house-dock="done">` + e(c.StepsDone) +
		`</button></div></div>`)
	b.WriteString(`</div>`)
	fmt.Fprintf(&b, `<div class="ha-house-frame" data-house-url="%s" data-frame-title="%s" hidden></div>`, e(url), e(c.FrameTitle))
	return b.String()
}

// houseDockIcon: a window over a Dock shelf, in the theme's colours.
const houseDockIcon = `<svg class="ha-house-invite-icon" viewBox="0 0 40 40" aria-hidden="true">` +
	`<rect class="ha-hm-win" x="7" y="5" width="26" height="20" rx="3"/>` +
	`<rect class="ha-hm-hi" x="7" y="5" width="26" height="5" rx="2"/>` +
	`<rect class="ha-hm-shelf" x="3" y="29" width="34" height="8" rx="3"/>` +
	`<rect class="ha-hm-app" x="8" y="30.5" width="5" height="5" rx="1.2"/>` +
	`<rect class="ha-hm-hi" x="17.5" y="30.5" width="5" height="5" rx="1.2"/>` +
	`<rect class="ha-hm-app" x="27" y="30.5" width="5" height="5" rx="1.2"/>` +
	`</svg>`

// houseMenuPicture draws Safari's menu bar with the File menu open and
// "Add to Dock…" highlighted. Widths are estimated from the character
// count; it is a picture, not a screenshot.
func houseMenuPicture(c houseCopy) string {
	const charW, itemH, fs = 6.4, 17, 11
	e := html.EscapeString
	textW := func(s string) float64 { return float64(len([]rune(s))) * charW }

	var b strings.Builder
	x := 12.0
	appW := textW("Safari") + 4
	barItems := fmt.Sprintf(`<text class="ha-hm-text ha-hm-bold" x="%.0f" y="15">Safari</text>`, x)
	x += appW + 14
	menuX := x - 7
	for i, m := range c.MenuBar {
		w := textW(m)
		if i == 0 {
			barItems += fmt.Sprintf(`<rect class="ha-hm-open" x="%.0f" y="3" width="%.0f" height="16" rx="4"/>`, x-7, w+14)
		}
		barItems += fmt.Sprintf(`<text class="ha-hm-text" x="%.0f" y="15">%s</text>`, x, e(m))
		x += w + 16
	}
	barW := x + 8

	menuW := 0.0
	for _, it := range c.MenuItems {
		menuW = max(menuW, textW(it)+28)
	}
	y := 26.0
	var items strings.Builder
	for _, it := range c.MenuItems {
		if it == "" {
			fmt.Fprintf(&items, `<line class="ha-hm-sep" x1="%.0f" x2="%.0f" y1="%.1f" y2="%.1f"/>`, menuX+8, menuX+menuW-8, y+4.5, y+4.5)
			y += 9
			continue
		}
		cls := "ha-hm-text"
		if it == c.MenuAdd {
			fmt.Fprintf(&items, `<rect class="ha-hm-sel" x="%.0f" y="%.0f" width="%.0f" height="%d" rx="4"/>`, menuX+4, y, menuW-8, itemH)
			cls = "ha-hm-text ha-hm-seltext"
		} else {
			cls += " ha-hm-dim"
		}
		fmt.Fprintf(&items, `<text class="%s" x="%.0f" y="%.0f">%s</text>`, cls, menuX+12, y+12.5, e(it))
		y += itemH
	}
	menuH := y - 22 + 4
	w, h := max(barW, menuX+menuW+12), y+10

	fmt.Fprintf(&b, `<svg class="ha-house-menu" viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f" aria-hidden="true" font-size="%d">`, w, h, w, h, fs)
	fmt.Fprintf(&b, `<rect class="ha-hm-bar" x="0" y="0" width="%.0f" height="22" rx="6"/>`, w)
	b.WriteString(barItems)
	fmt.Fprintf(&b, `<rect class="ha-hm-menu" x="%.0f" y="22" width="%.0f" height="%.0f" rx="6"/>`, menuX, menuW, menuH)
	b.WriteString(items.String())
	b.WriteString(`</svg>`)
	return b.String()
}

// houseDecideJS holds the two decisions of the house script as pure
// functions, so a test can run them under node (TestHouseScript_Decisions).
//
// haHouseInitialView: the stored choice wins; with none (or junk) a first
// visit opens on 3D, except on a narrow screen.
//
// haHouseShouldInvite: Safari 17+ on a Mac (not an iPad, which reports
// Macintosh too but has touch points; not Chrome, Firefox, Edge or Opera,
// which all say Safari/), not already running as a Dock web app, and not
// dismissed in this browser.
const houseDecideJS = `function haHouseInitialView(stored,narrow){if(stored==='plan'||stored==='3d')return stored;return narrow?'plan':'3d';}` +
	`function haHouseShouldInvite(ua,maxTouchPoints,standalone,displayStandalone,dismissed){if(dismissed||standalone===true||displayStandalone)return false;ua=String(ua||'');if(ua.indexOf('Macintosh')<0||ua.indexOf('Safari/')<0)return false;if(/Chrome|Chromium|CriOS|FxiOS|Firefox|Edg|OPR/.test(ua))return false;if(maxTouchPoints!==0)return false;var m=/Version\/(\d+)/.exec(ua);return !!m&&+m[1]>=17;}`

// houseScript runs in the widget's onerror payload after the bootstrap.
// It is guarded per root, so running it twice adds nothing. «3D» creates
// the iframe if it is not there yet (never a second one) and hides the
// map; «Plan» removes the iframe, which frees its WebGL context, so the
// 3D page is never requested and nothing renders while «Plan» is shown.
// It starts with ';' because the payload before it ends in '(this)'.
// Every storage access is in a try: a blocked or private-mode store just
// means no remembered choice.
const houseScript = `;(function(img){var root=img.closest('.ha-widget');if(!root||root.dataset.houseBooted)return;root.dataset.houseBooted='1';` +
	houseDecideJS +
	`var sw=root.querySelector('.ha-house-switch'),frame=root.querySelector('.ha-house-frame'),invite=root.querySelector('.ha-house-invite');if(!sw||!frame)return;` +
	`function get(k){try{return localStorage.getItem(k);}catch(e){return null;}}` +
	`function put(k,v){try{localStorage.setItem(k,v);}catch(e){}}` +
	`function mq(q){try{return matchMedia(q).matches;}catch(e){return false;}}` +
	`function show(view){root.dataset.houseView=view;sw.querySelectorAll('[data-house-view]').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.houseView===view?'true':'false');});var f=frame.querySelector('iframe');if(view==='3d'){if(!f){f=document.createElement('iframe');f.setAttribute('src',frame.dataset.houseUrl);f.setAttribute('title',frame.dataset.frameTitle||'');f.setAttribute('loading','eager');f.setAttribute('allow','fullscreen');frame.appendChild(f);}frame.hidden=false;}else{if(f)f.remove();frame.hidden=true;}}` +
	`show(haHouseInitialView(get('ha-house-view'),mq('(max-width: 600px)')));` +
	`sw.addEventListener('click',function(e){var b=e.target.closest('[data-house-view]');if(!b)return;var v=b.dataset.houseView==='3d'?'3d':'plan';put('ha-house-view',v);if(root.dataset.houseView!==v)show(v);});` +
	`if(invite&&haHouseShouldInvite(navigator.userAgent,navigator.maxTouchPoints,navigator.standalone,mq('(display-mode: standalone)'),get('ha-house-dock')==='dismissed')){invite.hidden=false;invite.addEventListener('click',function(e){var b=e.target.closest('[data-house-dock]');if(!b)return;if(b.dataset.houseDock==='add'){invite.dataset.step='steps';return;}put('ha-house-dock','dismissed');invite.hidden=true;});}` +
	`})(this)`

// houseCSS styles the switch like the section head around it, the frame
// at Ruling 14's size, and the invite like a room card. Only theme
// variables, so it follows Glance's light and dark themes.
const houseCSS = `
	.ha-house-switch{margin-left:auto;display:inline-flex;gap:2px;padding:2px;border-radius:7px;
	  border:1px solid var(--color-widget-content-border);background:var(--color-widget-background-highlight)}
	.ha-house-switch button{appearance:none;-webkit-appearance:none;border:0;margin:0;background:none;cursor:pointer;
	  font:inherit;font-size:.72em;letter-spacing:.06em;text-transform:uppercase;line-height:1;
	  color:var(--color-text-subdue);padding:5px 11px;border-radius:5px;transition:background .15s,color .15s}
	.ha-house-switch button:hover{color:var(--color-text-highlight)}
	.ha-house-switch button[aria-pressed="true"]{background:var(--color-primary);color:var(--color-widget-background)}
	.ha-house-switch button:focus-visible{outline:2px solid var(--color-primary);outline-offset:1px}
	.ha-widget:has(> .ha-fp-edit) > .ha-section-head{padding-right:26px}

	.ha-widget[data-house-view="3d"] > :is(.ha-fp-layout,.ha-rooms,.ha-empty,.ha-fp-edit){display:none}
	.ha-house-frame[hidden]{display:none}
	.ha-house-frame iframe{display:block;width:100%;height:clamp(520px,72vh,760px);border:0;border-radius:8px;background:#0a0e1a}

	.ha-house-invite{background:var(--color-widget-background-highlight);border:1px solid var(--color-widget-content-border);
	  border-radius:8px;padding:12px 14px}
	.ha-house-invite[hidden]{display:none}
	.ha-house-invite-ask,.ha-house-invite-steps{display:flex;align-items:center;gap:14px;flex-wrap:wrap}
	.ha-house-invite[data-step="ask"] .ha-house-invite-steps,.ha-house-invite[data-step="steps"] .ha-house-invite-ask{display:none}
	.ha-house-invite-icon{width:40px;height:40px;flex:none}
	.ha-house-invite-text{flex:1 1 220px;display:flex;flex-direction:column;gap:3px;font-size:.85em;color:var(--color-text-base)}
	.ha-house-invite-text b{font-size:13.5px;font-weight:600;color:var(--color-text-highlight)}
	.ha-house-invite-actions{display:flex;gap:8px;flex-wrap:wrap}
	.ha-house-invite-actions button{appearance:none;-webkit-appearance:none;cursor:pointer;font:inherit;font-size:.85em;
	  padding:6px 12px;border-radius:6px;border:1px solid var(--color-widget-content-border);background:none;color:var(--color-text-base)}
	.ha-house-invite-actions button:hover{color:var(--color-text-highlight);border-color:var(--color-text-subdue)}
	.ha-house-invite-actions .ha-house-primary{background:var(--color-primary);border-color:var(--color-primary);color:var(--color-widget-background)}
	.ha-house-invite-actions .ha-house-primary:hover{color:var(--color-widget-background);filter:brightness(1.08)}
	.ha-house-menu{flex:none;max-width:100%;height:auto;font-family:inherit}
	.ha-hm-bar{fill:var(--color-widget-background);stroke:var(--color-widget-content-border)}
	.ha-hm-menu{fill:var(--color-widget-background);stroke:var(--color-widget-content-border)}
	.ha-hm-open{fill:color-mix(in srgb,var(--color-text-subdue) 22%,transparent)}
	.ha-hm-sel{fill:var(--color-primary)}
	.ha-hm-sep{stroke:var(--color-widget-content-border)}
	.ha-hm-text{fill:var(--color-text-highlight)}
	.ha-hm-dim{fill:var(--color-text-base)}
	.ha-hm-bold{font-weight:700}
	.ha-hm-seltext{fill:var(--color-widget-background);font-weight:600}
	.ha-hm-win{fill:var(--color-widget-background);stroke:var(--color-text-subdue)}
	.ha-hm-shelf{fill:color-mix(in srgb,var(--color-text-subdue) 30%,transparent)}
	.ha-hm-app{fill:var(--color-text-subdue)}
	.ha-hm-hi{fill:var(--color-primary)}
`
