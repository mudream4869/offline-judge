//go:build js && wasm

package main

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgcomp/tcutil"
	"github.com/voilelab/toolgui/toolgui/tgframe"
)

// Settings sets the problem sources, default code, display and backup, one tab each.
func Settings(p *tgframe.Params) error {
	tgcomp.Title(p.Main, "設定")
	srcTab, tmplTab, viewTab, backupTab := tgcomp.Tab4(p.Main, "題目來源", "預設程式碼", "顯示", "備份")
	cf := newConfirm(p, "settings")
	done := cf.confirmed()
	sourcesSection(p, srcTab, cf, done)
	templateSection(tmplTab, cf, done)
	displaySection(viewTab)
	backupSection(backupTab)
	cf.draw()
	return nil
}

// sourcesSection lists, adds and removes problem sources.
func sourcesSection(p *tgframe.Params, c *tgframe.Container, cf *confirmDialog, done string) {
	// Clicks first, so everything below sees the new list.
	urls := sourceURLs()
	for _, url := range urls {
		if done == "rm:"+url {
			setSourceURLs(slices.DeleteFunc(slices.Clone(urls), func(u string) bool { return u == url }))
		}
		if tgcomp.ButtonClicked(c, "移除", removeConf(url)) {
			cf.ask("rm:"+url, "移除來源「"+sourceLabel(url)+"」？之後可以再加回來。", "移除")
		}
	}
	for _, r := range recommended {
		if tgcomp.ButtonClicked(c, "加入", recommendConf(r.url)) {
			addSource(c, r.url)
		}
	}
	resetConf := &tgcomp.ButtonConf{ID: "source_reset"}
	if done == "src_reset" {
		setSourceURLs([]string{defaultSource})
	}
	if tgcomp.ButtonClicked(c, "還原預設", resetConf) {
		cf.ask("src_reset", "還原成只有 Offline Judge 題庫？其他來源都會被移除。", "還原")
	}
	urls = sourceURLs()

	tgcomp.Subtitle(c, "目前來源")
	tgcomp.Caption(c, "題目列表合併顯示所有來源的題目")
	if len(urls) == 0 {
		tgcomp.MessageWarning(c, "沒有任何來源，題目列表會是空的；可以從下方的推薦來源加入")
	}
	var loaded []string
	for _, url := range urls {
		box := tgcomp.Box(c, &tgcomp.BoxConf{Base: tgframe.Base{ID: "src_" + url}})
		tgcomp.Text(box, sourceLabel(url))
		if s := openSet(box, p.Context, url); s != nil {
			loaded = append(loaded, url)
			es := s.Entries()
			cached := 0
			for _, e := range es {
				if e.Cached {
					cached++
				}
			}
			tgcomp.Caption(box, fmt.Sprintf("commit %s，上次檢查 %s，共 %d 題，%d 題可離線",
				s.Commit()[:7], s.Checked().Format("2006-01-02 15:04"), len(es), cached))
		}
		tgcomp.Button(box, "移除", removeConf(url))
	}
	sourceActions(p, c, loaded)

	tgcomp.Subtitle(c, "新增來源")
	// Cleared after each add, keyed by the list length.
	url := tgcomp.Textbox(c, "GitHub 網址", &tgcomp.TextboxConf{
		Base:        tgframe.Base{ID: "source_new"},
		ResetKey:    strconv.Itoa(len(urls)),
		Placeholder: "https://github.com/<owner>/<repo>/tree/<分支>/<資料夾>",
	})
	tgcomp.Caption(c, "可以是本站格式的題庫、放了多個 Kattis 題目包的資料夾，或單一 Kattis 題目包；分支名稱不能含 /")
	if tgcomp.Button(c, "新增", &tgcomp.ButtonConf{ID: "source_add"}) && url != "" {
		addSource(c, url)
		app.RerunPage("settings")
	}

	tgcomp.Subtitle(c, "推薦來源")
	for _, r := range recommended {
		box := tgcomp.Box(c, &tgcomp.BoxConf{Base: tgframe.Base{ID: "rec_" + r.url}})
		tgcomp.Text(box, r.name)
		tgcomp.Caption(box, r.note+"："+r.url)
		if slices.Contains(urls, r.url) {
			tgcomp.Caption(box, "已加入")
		} else {
			tgcomp.Button(box, "加入", recommendConf(r.url))
		}
	}
	tgcomp.Button(c, "還原預設", resetConf)
	tgcomp.Caption(c, "還原成只有 Offline Judge 題庫")
}

// displaySection sets display options.
func displaySection(c *tgframe.Container) {
	setShowSolutionTags(tgcomp.Checkbox(c, "顯示解法標籤（可能暴雷）", &tgcomp.CheckboxConf{
		Base:    tgframe.Base{ID: "show_solution_tags"},
		Default: showSolutionTags(),
	}))
	tgcomp.Caption(c, "解法標籤會提示要用的演算法，預設收合；開啟後顯示在題目列表與題目頁，也能用來篩選")
}

// templateSection edits each language's default code.
func templateSection(c *tgframe.Container, cf *confirmDialog, done string) {
	tgcomp.Caption(c, "新題目的編輯器一開始放這段程式碼，「還原預設程式碼」也還原成它；"+
		"函式題用題目附的 template")
	names := make([]string, len(langs))
	for i, l := range langs {
		names[i] = l.name
	}
	li := tgcomp.Select(c, "語言", names, (&tgcomp.SelectConf{
		Base: tgframe.Base{ID: "template_lang"},
	}).SetDefault(memo.getLang()))
	if li == nil {
		return
	}
	lg := langs[*li]
	key := templateKey(lg)
	resetConf := &tgcomp.ButtonConf{ID: "template_reset_" + lg.id}
	if tgcomp.ButtonClicked(c, "還原成內建範本", resetConf) {
		cf.ask("tmpl_reset:"+lg.id, lg.name+" 的預設程式碼會還原成內建範本，無法復原。確定嗎？", "還原")
	}
	reset := done == "tmpl_reset:"+lg.id
	if reset {
		setUserTemplate(lg, lg.code)
	}
	code := tgcomp.CodeInput(c, lg.name+" 預設程式碼", &tgcomp.CodeInputConf{
		ID:        key,
		Language:  lg.hl,
		Height:    10,
		MaxHeight: 30,
		Default:   userTemplate(lg),
		ResetKey:  codeResetKey(key, reset),
	})
	setUserTemplate(lg, code)
	tgcomp.Button(c, "還原成內建範本", resetConf)
	tgcomp.Caption(c, "已經改過的題目不會跟著變；要套用到某一題，在題目頁按「還原預設程式碼」")
}

// sourceActions checks for updates and downloads, for every loaded source.
func sourceActions(p *tgframe.Params, c *tgframe.Container, urls []string) {
	if len(urls) == 0 {
		return
	}
	if tgcomp.Button(c, "檢查更新", &tgcomp.ButtonConf{ID: "source_refresh"}) {
		done := tgcomp.Spinner(c, "檢查中…")
		for _, url := range urls {
			if err := loadedSet(url).Refresh(p.Context); err != nil {
				tgcomp.MessageDanger(c, "檢查 "+sourceLabel(url)+" 更新失敗："+err.Error())
			}
		}
		done()
		app.RerunPage("settings")
	}
	if tgcomp.Button(c, "全部下載（離線使用）", &tgcomp.ButtonConf{ID: "source_download"}) {
		st := tgcomp.Status(c, "下載中…")
		failed := false
		for _, url := range urls {
			label := sourceLabel(url)
			err := loadedSet(url).DownloadAll(p.Context, func(done, total int) {
				st.Update(fmt.Sprintf("下載 %s 中 %d/%d", label, done, total))
			})
			if err != nil {
				failed = true
				tgcomp.MessageDanger(c, label+"："+err.Error())
			}
		}
		if failed {
			st.Error("下載失敗")
		} else {
			st.Complete("下載完成")
		}
	}
}

func removeConf(url string) *tgcomp.ButtonConf {
	return &tgcomp.ButtonConf{ID: "src_rm_" + url, Color: tcutil.ColorDanger}
}

func recommendConf(url string) *tgcomp.ButtonConf {
	return &tgcomp.ButtonConf{ID: "src_add_" + url}
}

// showSolutionTags reports whether solution tags are shown, a setting.
func showSolutionTags() bool {
	return memo.getText("show_solution_tags", "") != ""
}

func setShowSolutionTags(show bool) {
	v := ""
	if show {
		v = "1"
	}
	memo.setText("show_solution_tags", v)
}
