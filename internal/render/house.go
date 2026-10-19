package render

import (
	"fmt"
	"html"
	"strings"
)

// The «Plan | 3D» switch, the lazy iframe holding the House Control 3D
// page, and the invite to install House Control on a Mac. None of it is
// rendered unless WidgetData.HouseURL is set (public_url configured and a
// plan uploaded), so a widget without a plan is byte-for-byte what it was
// before; the invite also needs WidgetData.HouseInstall.
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
	Plan, ThreeD                                                     string
	InviteTitle, InviteBody, InviteInstall, InviteLater, InviteClose string
	// Steps are the steps card's lines; Copy is its button, Copied and
	// Failed what the button says after, CopyHint the line beside the
	// command when it could not be put on the clipboard.
	Steps                          []string
	Copy, Copied, Failed, CopyHint string
	StepsDone                      string
	CommandLabel                   string
	FrameTitle                     string
	Terminal                       string // the Terminal window's title in the picture
}

func houseStrings(lang string) houseCopy {
	if lang == "ru" {
		return houseCopy{
			Plan:          "План",
			ThreeD:        "3D",
			InviteTitle:   "House Control на Mac",
			InviteBody:    "3D-дом в строке меню и в «чёлке», уже настроенный на ваш дом.",
			InviteInstall: "Установить",
			InviteLater:   "Напомнить завтра",
			InviteClose:   "Не показывать",
			Steps: []string{
				"1. Откройте Терминал (⌘ Пробел → «Терминал»).",
				"2. Нажмите «Скопировать команду».",
				"3. Вставьте её в Терминал (⌘V) и нажмите Enter.",
			},
			Copy:         "Скопировать команду",
			Copied:       "Скопировано ✓",
			Failed:       "Не получилось — обновите страницу",
			CopyHint:     "Скопируйте: ⌘C",
			StepsDone:    "Готово",
			CommandLabel: "Команда установки",
			FrameTitle:   "Дом в 3D",
			Terminal:     "Терминал",
		}
	}
	return houseCopy{
		Plan:          "Plan",
		ThreeD:        "3D",
		InviteTitle:   "House Control on your Mac",
		InviteBody:    "The 3D home in the menu bar and in the notch, already set up for your home.",
		InviteInstall: "Install",
		InviteLater:   "Remind me tomorrow",
		InviteClose:   "Don't show again",
		Steps: []string{
			"1. Open Terminal (⌘ Space → “Terminal”).",
			"2. Click “Copy command”.",
			"3. Paste it into Terminal (⌘V) and press Return.",
		},
		Copy:         "Copy command",
		Copied:       "Copied ✓",
		Failed:       "That didn't work — reload the page",
		CopyHint:     "Copy it: ⌘C",
		StepsDone:    "Done",
		CommandLabel: "Install command",
		FrameTitle:   "Home in 3D",
		Terminal:     "Terminal",
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

// renderHouseParts is the invite card (only with installs on, and hidden
// until the script decides this browser should see it) and the empty
// frame container the script creates the iframe in. Both are siblings of
// the map/cards. The card asks the install code from codeURL, under the
// same house URL the iframe uses.
func renderHouseParts(url string, install bool, c houseCopy) string {
	e := html.EscapeString
	var b strings.Builder
	if install {
		codeURL := strings.TrimRight(url, "/") + "/install/code"
		fmt.Fprintf(&b, `<div class="ha-house-invite" hidden data-step="ask" data-code-url="%s" data-copied="%s" data-failed="%s">`,
			e(codeURL), e(c.Copied), e(c.Failed))
		b.WriteString(`<button type="button" class="ha-house-invite-close" data-house-act="close" aria-label="` + e(c.InviteClose) + `" title="` + e(c.InviteClose) + `">` +
			`<svg viewBox="0 0 12 12" aria-hidden="true"><path d="M2.5 2.5l7 7M9.5 2.5l-7 7"/></svg></button>`)
		b.WriteString(`<div class="ha-house-invite-ask">` + houseMacIcon +
			`<div class="ha-house-invite-text"><b>` + e(c.InviteTitle) + `</b><span>` + e(c.InviteBody) + `</span></div>` +
			`<div class="ha-house-invite-actions"><button type="button" class="ha-house-primary" data-house-act="install">` + e(c.InviteInstall) +
			`</button><button type="button" data-house-act="later">` + e(c.InviteLater) + `</button></div></div>`)
		b.WriteString(`<div class="ha-house-invite-steps">` + houseTerminalPicture(c) + `<div class="ha-house-invite-text"><b>` + e(c.InviteTitle) + `</b>`)
		for _, s := range c.Steps {
			b.WriteString(`<span>` + e(s) + `</span>`)
		}
		b.WriteString(`<div class="ha-house-cmd" hidden><input type="text" readonly spellcheck="false" aria-label="` + e(c.CommandLabel) + `">` +
			`<span hidden>` + e(c.CopyHint) + `</span></div></div>` +
			`<div class="ha-house-invite-actions"><button type="button" class="ha-house-primary" data-house-act="copy">` + e(c.Copy) +
			`</button><button type="button" data-house-act="done">` + e(c.StepsDone) + `</button></div></div>`)
		b.WriteString(`</div>`)
	}
	fmt.Fprintf(&b, `<div class="ha-house-frame" data-house-url="%s" data-frame-title="%s" hidden></div>`, e(url), e(c.FrameTitle))
	return b.String()
}

// houseMacIcon: a Mac screen with its menu bar and a house on it, in the
// theme's colours.
const houseMacIcon = `<svg class="ha-house-invite-icon" viewBox="0 0 40 40" aria-hidden="true">` +
	`<rect class="ha-ht-win" x="4" y="6" width="32" height="22" rx="3"/>` +
	`<rect class="ha-ht-hi" x="4" y="6" width="32" height="4" rx="2"/>` +
	`<path class="ha-ht-hi" d="M20 13.5l-6 5h1.8v5.5h8.4v-5.5H26z"/>` +
	`<path class="ha-ht-stand" d="M16 28h8l1.5 5h-11z"/>` +
	`<rect class="ha-ht-stand" x="12" y="33" width="16" height="2" rx="1"/>` +
	`</svg>`

// houseTerminalPicture draws a Terminal window with the install command
// typed at the prompt, highlighted. The command is a stand-in, shortened
// to fit; it is a picture, not a screenshot.
func houseTerminalPicture(c houseCopy) string {
	const w, h, charW, fs = 236.0, 92.0, 6.0, 10
	const prompt, command = "~ %", "curl -fsSL '…/install/…' | sh"
	cmdX := 12 + float64(len([]rune(prompt))+1)*charW
	cmdW := float64(len([]rune(command)))*charW + 8
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="ha-house-term" viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f" aria-hidden="true" font-size="%d">`, w, h, w, h, fs)
	fmt.Fprintf(&b, `<rect class="ha-ht-win" x="0.5" y="0.5" width="%.0f" height="%.0f" rx="7"/>`, w-1, h-1)
	fmt.Fprintf(&b, `<path class="ha-ht-bar" d="M0.5 22V7.5a7 7 0 0 1 7-7h%.0fa7 7 0 0 1 7 7V22z"/>`, w-15)
	b.WriteString(`<circle fill="#ff5f57" cx="13" cy="11.5" r="4.5"/><circle fill="#febc2e" cx="27" cy="11.5" r="4.5"/><circle fill="#28c840" cx="41" cy="11.5" r="4.5"/>`)
	fmt.Fprintf(&b, `<text class="ha-ht-text ha-ht-title" x="%.0f" y="15" text-anchor="middle">%s</text>`, w/2, html.EscapeString(c.Terminal))
	b.WriteString(`<g class="ha-ht-mono">`)
	fmt.Fprintf(&b, `<text class="ha-ht-text ha-ht-dim" x="12" y="50">%s</text>`, prompt)
	fmt.Fprintf(&b, `<rect class="ha-ht-sel" x="%.0f" y="38" width="%.0f" height="17" rx="3"/>`, cmdX-4, cmdW)
	fmt.Fprintf(&b, `<text class="ha-ht-text ha-ht-seltext" x="%.0f" y="50">%s</text>`, cmdX, html.EscapeString(command))
	fmt.Fprintf(&b, `<text class="ha-ht-text ha-ht-dim" x="12" y="72">%s</text>`, prompt)
	fmt.Fprintf(&b, `<rect class="ha-ht-cursor" x="%.0f" y="62" width="6" height="12"/>`, cmdX)
	b.WriteString(`</g></svg>`)
	return b.String()
}

// houseDecideJS holds the decisions of the house script as pure
// functions, so a test can run them under node (TestHouseScript_Decisions).
//
// haHouseInitialView: the stored choice wins; with none (or junk) a first
// visit opens on 3D, except on a narrow screen.
//
// haHouseShouldInvite: a Mac in any browser (not an iPad, which reports
// Macintosh too but has touch points), unless this browser dismissed the
// invite or started an install less than 24 hours ago ('started:<epoch
// ms>'; after that the person may need a fresh command), and not before a
// «remind me tomorrow» time (epoch ms) has come.
//
// haHouseDoneState is the invite's stored state after «Готово» in the
// steps: after a copy (started) the install worked, so the invite is
// dismissed for good; before one it only hides for this page view (null:
// nothing stored). Only a started install left without «Готово» comes back.
//
// haHouseCommand is the Terminal line for an install script's path (or
// URL), against the page's origin. globalThis.URL, because an onerror
// handler runs with the document in scope and a bare URL is document.URL.
//
// haHouseCodeFresh: an install code lives 30 minutes on the server; one
// fetched 25 or more minutes ago is not reused.
const houseDecideJS = `function haHouseInitialView(stored,narrow){if(stored==='plan'||stored==='3d')return stored;return narrow?'plan':'3d';}` +
	`function haHouseShouldInvite(ua,maxTouchPoints,state,remindAt,now){if(state==='dismissed')return false;if(String(state).indexOf('started:')===0&&now-state.slice(8)<864e5)return false;ua=String(ua||'');if(ua.indexOf('Macintosh')<0||maxTouchPoints!==0)return false;var r=+remindAt;return !(r>now);}` +
	`function haHouseDoneState(state){return String(state).indexOf('started:')===0?'dismissed':null;}` +
	`function haHouseCommand(path,origin){return "curl -fsSL '"+new globalThis.URL(path,origin).href+"' | sh";}` +
	`function haHouseCodeFresh(at,now){return at>0&&now-at<25*60000;}`

// houseScript runs in the widget's onerror payload after the bootstrap.
// It is guarded per root, so running it twice adds nothing. «3D» creates
// the iframe if it is not there yet (never a second one) and hides the
// map; «Plan» removes the iframe, which frees its WebGL context, so the
// 3D page is never requested and nothing renders while «Plan» is shown.
// It starts with ';' because the payload before it ends in '(this)'.
// Every storage access is in a try: a blocked or private-mode store just
// means no remembered choice.
//
// The invite fetches an install code when «Install» opens the steps, so
// that «Copy command» can put the line on the clipboard inside the click
// (Safari refuses a clipboard write that waits on a fetch); a code 25
// minutes old is fetched anew on the next click. Where there is no
// clipboard API (a plain-http LAN origin is not a secure context) or it
// refuses, the command is shown selected in a read-only field, copied with
// execCommand where that still works, else with ⌘C, which counts as
// started too. A 404 means installs are off: the card goes away.
const houseScript = `;(function(img){var root=img.closest('.ha-widget');if(!root||root.dataset.houseBooted)return;root.dataset.houseBooted='1';` +
	houseDecideJS +
	`var sw=root.querySelector('.ha-house-switch'),frame=root.querySelector('.ha-house-frame'),invite=root.querySelector('.ha-house-invite');if(!sw||!frame)return;` +
	`function get(k){try{return localStorage.getItem(k);}catch(e){return null;}}` +
	`function put(k,v){try{localStorage.setItem(k,v);}catch(e){}}` +
	`function mq(q){try{return matchMedia(q).matches;}catch(e){return false;}}` +
	`function show(view){root.dataset.houseView=view;sw.querySelectorAll('[data-house-view]').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.houseView===view?'true':'false');});var f=frame.querySelector('iframe');if(view==='3d'){if(!f){f=document.createElement('iframe');f.setAttribute('src',frame.dataset.houseUrl);f.setAttribute('title',frame.dataset.frameTitle||'');f.setAttribute('loading','eager');f.setAttribute('allow','fullscreen');frame.appendChild(f);}frame.hidden=false;}else{if(f)f.remove();frame.hidden=true;}}` +
	`show(haHouseInitialView(get('ha-house-view'),mq('(max-width: 600px)')));` +
	`sw.addEventListener('click',function(e){var b=e.target.closest('[data-house-view]');if(!b)return;var v=b.dataset.houseView==='3d'?'3d':'plan';put('ha-house-view',v);if(root.dataset.houseView!==v)show(v);});` +
	`if(invite&&haHouseShouldInvite(navigator.userAgent,navigator.maxTouchPoints,get('ha-house-invite'),get('ha-house-invite-remind'),Date.now())){invite.hidden=false;` +
	`var copyBtn=invite.querySelector('[data-house-act="copy"]'),box=invite.querySelector('.ha-house-cmd'),field=box.querySelector('input'),hint=box.querySelector('span'),cmd=null,cmdAt=0,pending=null;` +
	`function copied(){copyBtn.textContent=invite.dataset.copied;put('ha-house-invite','started:'+Date.now());}` +
	`function fetchCmd(){if(!pending)pending=fetch(invite.dataset.codeUrl,{method:'POST',credentials:'same-origin',cache:'no-store'}).then(function(r){if(r.status===404)invite.hidden=true;if(!r.ok)throw new Error(r.status);return r.json();}).then(function(j){cmd=haHouseCommand(j.path,location.origin);cmdAt=Date.now();pending=null;return cmd;},function(e){pending=null;throw e;});return pending;}` +
	`function manual(c){box.hidden=false;field.value=c;field.focus();field.select();var ok=false;try{ok=document.execCommand('copy');}catch(e){}hint.hidden=ok;if(ok)copied();}` +
	`function copy(c){field.value=c;var cb=navigator.clipboard;if(window.isSecureContext&&cb&&cb.writeText)cb.writeText(c).then(copied,function(){manual(c);});else manual(c);}` +
	`field.addEventListener('copy',copied);field.addEventListener('focus',function(){field.select();});` +
	`invite.addEventListener('click',function(e){var b=e.target.closest('[data-house-act]');if(!b)return;var act=b.dataset.houseAct;` +
	`if(act==='install'){invite.dataset.step='steps';if(!haHouseCodeFresh(cmdAt,Date.now()))fetchCmd().catch(function(){});return;}` +
	`if(act==='copy'){if(cmd&&haHouseCodeFresh(cmdAt,Date.now()))copy(cmd);else fetchCmd().then(copy,function(){if(!invite.hidden)copyBtn.textContent=invite.dataset.failed;});return;}` +
	`if(act==='later')put('ha-house-invite-remind',String(Date.now()+864e5));else if(act==='close')put('ha-house-invite','dismissed');else if(act==='done'){var ds=haHouseDoneState(get('ha-house-invite'));if(ds)put('ha-house-invite',ds);}invite.hidden=true;});}` +
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

	.ha-house-invite{position:relative;background:var(--color-widget-background-highlight);border:1px solid var(--color-widget-content-border);
	  border-radius:8px;padding:12px 38px 12px 14px}
	.ha-house-invite[hidden]{display:none}
	.ha-house-invite-close{position:absolute;top:6px;right:6px;appearance:none;-webkit-appearance:none;border:0;margin:0;padding:6px;
	  background:none;cursor:pointer;border-radius:5px;line-height:0;color:var(--color-text-subdue)}
	.ha-house-invite-close:hover{color:var(--color-text-highlight);background:color-mix(in srgb,var(--color-text-subdue) 18%,transparent)}
	.ha-house-invite-close:focus-visible{outline:2px solid var(--color-primary);outline-offset:1px}
	.ha-house-invite-close svg{width:12px;height:12px;fill:none;stroke:currentColor;stroke-width:1.6;stroke-linecap:round}
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
	.ha-house-cmd{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-top:5px}
	.ha-house-cmd[hidden],.ha-house-cmd span[hidden]{display:none}
	.ha-house-cmd input{flex:1 1 200px;min-width:0;font:12px ui-monospace,SFMono-Regular,Menlo,monospace;padding:5px 7px;border-radius:5px;
	  border:1px solid var(--color-widget-content-border);background:var(--color-widget-background);color:var(--color-text-highlight)}
	.ha-house-cmd span{color:var(--color-text-highlight);font-weight:600}
	.ha-house-term{flex:none;max-width:100%;height:auto;font-family:inherit}
	.ha-ht-win{fill:var(--color-widget-background);stroke:var(--color-widget-content-border)}
	.ha-ht-bar{fill:color-mix(in srgb,var(--color-text-subdue) 14%,transparent)}
	.ha-ht-text{fill:var(--color-text-highlight)}
	.ha-ht-title{font-weight:600}
	.ha-ht-mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
	.ha-ht-dim{fill:var(--color-text-base)}
	.ha-ht-sel{fill:var(--color-primary)}
	.ha-ht-seltext{fill:var(--color-widget-background);font-weight:600}
	.ha-ht-cursor{fill:var(--color-text-subdue)}
	.ha-ht-hi{fill:var(--color-primary)}
	.ha-ht-stand{fill:var(--color-text-subdue)}
`
