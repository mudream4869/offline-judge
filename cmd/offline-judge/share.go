//go:build js && wasm

package main

import (
	"errors"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/share"
	"github.com/mudream4869/offline-judge/problems"
)

// shareLink returns a link that opens pr with code in lg.
func shareLink(pr *problems.Problem, lg *lang, code string) (string, error) {
	c := share.Code{Lang: lg.id, Code: code}
	if ref, ok := refOf(pr.ID); ok && ref.url != defaultSource {
		c.Source = ref.url
	}
	q := url.Values{"id": {pr.ID}, "share": {share.Encode(c)}}.Encode()
	// A longer query fails the page that opens it.
	if len(q) > tgframe.MaxQuerySize {
		return "", errors.New("程式碼太長，壓縮後仍超過網址能放的 8 KB")
	}
	return appBase() + "#/problems?" + q, nil
}

// appBase is the URL the app is served at, ending in "/".
func appBase() string {
	return strings.TrimSuffix(assetURL("x"), "assets/x")
}

// shareButton offers a link to code, below the editor.
func shareButton(c *tgframe.Container, pr *problems.Problem, lg *lang, code, key string) {
	if !tgcomp.Button(c, "分享程式碼", &tgcomp.ButtonConf{ID: "share_" + key}) {
		return
	}
	link, err := shareLink(pr, lg, code)
	if err != nil {
		tgcomp.MessageWarning(c, err.Error())
		return
	}
	tgcomp.Caption(c, "複製下面的連結：程式碼壓縮後放在網址裡，不會上傳到任何地方")
	tgcomp.Code(c, link, &tgcomp.CodeConf{Language: "text"})
	tgcomp.Link(c, "開啟分享連結", link)
}

// sharedBox shows the code a share link carries, if any, with a button that
// loads it into the editor.
func sharedBox(p *tgframe.Params, pr *problems.Problem) {
	s := p.Query.Get("share")
	if s == "" {
		return
	}
	c, err := share.Decode(s)
	if err != nil {
		tgcomp.MessageWarning(p.Main, err.Error())
		return
	}
	li := slices.IndexFunc(langs, func(l *lang) bool { return l.id == c.Lang })
	if li < 0 {
		tgcomp.MessageWarning(p.Main, "分享的程式碼用了不支援的語言："+c.Lang)
		return
	}
	lg := langs[li]
	box := tgcomp.Box(p.Main, &tgcomp.BoxConf{Base: tgframe.Base{ID: "shared"}})
	tgcomp.Subtitle(box, "分享的程式碼（"+lg.name+"）")
	tgcomp.Code(box, c.Code, &tgcomp.CodeConf{Language: lg.hl})
	tgcomp.Caption(box, "載入會取代你這題 "+lg.name+" 的程式碼；交過的程式碼仍在紀錄裡")
	if tgcomp.Button(box, "載入到編輯器", &tgcomp.ButtonConf{ID: "shared_load"}) {
		key := lg.id + "_" + pr.ID
		memo.setLang(li)
		memo.setText("code_"+key, c.Code)
		codeResetKey(key, true)
		p.Navigate("problems", withoutShare(p.Query))
	}
}

// sharedSource offers to add the source a share link's problem comes from,
// when it isn't one yet. It reports whether it did.
func sharedSource(p *tgframe.Params) bool {
	c, err := share.Decode(p.Query.Get("share"))
	if err != nil || c.Source == "" || slices.Contains(sourceURLs(), c.Source) {
		return false
	}
	tgcomp.MessageInfo(p.Main, "分享的程式碼是「"+sourceLabel(c.Source)+"」的題目，這個來源還沒加入")
	if tgcomp.Button(p.Main, "加入這個題目來源", &tgcomp.ButtonConf{ID: "shared_source"}) {
		addSource(p.Main, c.Source)
		app.RerunPage("problems")
	}
	return true
}

func withoutShare(q url.Values) url.Values {
	q = maps.Clone(q)
	q.Del("share")
	return q
}
