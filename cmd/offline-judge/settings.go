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

// Settings sets the problem sources and display options.
func Settings(p *tgframe.Params) error {
	tgcomp.Title(p.Main, "設定")

	// Clicks first, so everything below sees the new list.
	urls := sourceURLs()
	for _, url := range urls {
		if tgcomp.ButtonClicked(p.Main, "移除", removeConf(url)) {
			setSourceURLs(slices.DeleteFunc(slices.Clone(urls), func(u string) bool { return u == url }))
		}
	}
	for _, r := range recommended {
		if tgcomp.ButtonClicked(p.Main, "加入", recommendConf(r.url)) {
			addSource(p.Main, r.url)
		}
	}
	resetConf := &tgcomp.ButtonConf{ID: "source_reset"}
	if tgcomp.ButtonClicked(p.Main, "還原預設", resetConf) {
		setSourceURLs([]string{defaultSource})
	}
	urls = sourceURLs()

	tgcomp.Subtitle(p.Main, "題目來源")
	tgcomp.Caption(p.Main, "題目列表合併顯示所有來源的題目")
	if len(urls) == 0 {
		tgcomp.MessageWarning(p.Main, "沒有任何來源，題目列表會是空的；可以從下方的推薦來源加入")
	}
	var loaded []string
	for _, url := range urls {
		box := tgcomp.Box(p.Main, &tgcomp.BoxConf{Base: tgframe.Base{ID: "src_" + url}})
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
	sourceActions(p, loaded)

	tgcomp.Subtitle(p.Main, "新增來源")
	// Cleared after each add, keyed by the list length.
	url := tgcomp.Textbox(p.Main, "GitHub 網址", &tgcomp.TextboxConf{
		Base:        tgframe.Base{ID: "source_new"},
		ResetKey:    strconv.Itoa(len(urls)),
		Placeholder: "https://github.com/<owner>/<repo>/tree/<分支>/<資料夾>",
	})
	tgcomp.Caption(p.Main, "可以是本站格式的題庫、放了多個 Kattis 題目包的資料夾，或單一 Kattis 題目包；分支名稱不能含 /")
	if tgcomp.Button(p.Main, "新增", &tgcomp.ButtonConf{ID: "source_add"}) && url != "" {
		addSource(p.Main, url)
		app.RerunPage("settings")
	}

	tgcomp.Subtitle(p.Main, "推薦來源")
	for _, r := range recommended {
		box := tgcomp.Box(p.Main, &tgcomp.BoxConf{Base: tgframe.Base{ID: "rec_" + r.url}})
		tgcomp.Text(box, r.name)
		tgcomp.Caption(box, r.note+"："+r.url)
		if slices.Contains(urls, r.url) {
			tgcomp.Caption(box, "已加入")
		} else {
			tgcomp.Button(box, "加入", recommendConf(r.url))
		}
	}
	tgcomp.Button(p.Main, "還原預設", resetConf)
	tgcomp.Caption(p.Main, "還原成只有 Offline Judge 題庫")

	templateSection(p)

	backupSection(p)

	tgcomp.Subtitle(p.Main, "顯示")
	setShowSolutionTags(tgcomp.Checkbox(p.Main, "顯示解法標籤（可能暴雷）", &tgcomp.CheckboxConf{
		Base:    tgframe.Base{ID: "show_solution_tags"},
		Default: showSolutionTags(),
	}))
	tgcomp.Caption(p.Main, "解法標籤會提示要用的演算法，預設收合；開啟後顯示在題目列表與題目頁，也能用來篩選")
	return nil
}

// templateSection edits each language's default code.
func templateSection(p *tgframe.Params) {
	tgcomp.Subtitle(p.Main, "預設程式碼")
	tgcomp.Caption(p.Main, "新題目的編輯器一開始放這段程式碼，「還原預設程式碼」也還原成它；"+
		"函式題用題目附的 template")
	names := make([]string, len(langs))
	for i, l := range langs {
		names[i] = l.name
	}
	li := tgcomp.Select(p.Main, "語言", names, (&tgcomp.SelectConf{
		Base: tgframe.Base{ID: "template_lang"},
	}).SetDefault(memo.getLang()))
	if li == nil {
		return
	}
	lg := langs[*li]
	key := templateKey(lg)
	resetConf := &tgcomp.ButtonConf{ID: "template_reset_" + lg.id}
	reset := tgcomp.ButtonClicked(p.Main, "還原成內建範本", resetConf)
	if reset {
		setUserTemplate(lg, lg.code)
	}
	code := tgcomp.CodeInput(p.Main, lg.name+" 預設程式碼", &tgcomp.CodeInputConf{
		ID:        key,
		Language:  lg.hl,
		Height:    10,
		MaxHeight: 30,
		Default:   userTemplate(lg),
		ResetKey:  codeResetKey(key, reset),
	})
	setUserTemplate(lg, code)
	tgcomp.Button(p.Main, "還原成內建範本", resetConf)
	tgcomp.Caption(p.Main, "已經改過的題目不會跟著變；要套用到某一題，在題目頁按「還原預設程式碼」")
}

// sourceActions checks for updates and downloads, for every loaded source.
func sourceActions(p *tgframe.Params, urls []string) {
	if len(urls) == 0 {
		return
	}
	if tgcomp.Button(p.Main, "檢查更新", &tgcomp.ButtonConf{ID: "source_refresh"}) {
		done := tgcomp.Spinner(p.Main, "檢查中…")
		for _, url := range urls {
			if err := loadedSet(url).Refresh(p.Context); err != nil {
				tgcomp.MessageDanger(p.Main, "檢查 "+sourceLabel(url)+" 更新失敗："+err.Error())
			}
		}
		done()
		app.RerunPage("settings")
	}
	if tgcomp.Button(p.Main, "全部下載（離線使用）", &tgcomp.ButtonConf{ID: "source_download"}) {
		st := tgcomp.Status(p.Main, "下載中…")
		failed := false
		for _, url := range urls {
			label := sourceLabel(url)
			err := loadedSet(url).DownloadAll(p.Context, func(done, total int) {
				st.Update(fmt.Sprintf("下載 %s 中 %d/%d", label, done, total))
			})
			if err != nil {
				failed = true
				tgcomp.MessageDanger(p.Main, label+"："+err.Error())
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
