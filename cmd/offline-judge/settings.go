//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/source"
)

// Settings sets the problem source.
func Settings(p *tgframe.Params) error {
	tgcomp.Title(p.Main, "設定")

	resetConf := &tgcomp.ButtonConf{ID: "source_reset"}
	if tgcomp.ButtonClicked(p.Main, "還原預設", resetConf) {
		memo.setText("source", defaultSource)
	}
	cur := sourceURL()
	url := tgcomp.Textbox(p.Main, "題目來源", &tgcomp.TextboxConf{
		Default:     cur,
		ResetKey:    cur,
		Placeholder: defaultSource,
	})
	tgcomp.Caption(p.Main, "GitHub 上的資料夾，例如 "+defaultSource+
		"；分支名稱不能含 /")
	if tgcomp.Button(p.Main, "套用", &tgcomp.ButtonConf{ID: "source_apply"}) {
		if _, err := source.Parse(url); err != nil {
			tgcomp.MessageDanger(p.Main, err.Error())
		} else {
			memo.setText("source", url)
		}
	}
	tgcomp.Button(p.Main, "還原預設", resetConf)

	tgcomp.Subtitle(p.Main, "顯示")
	setShowSolutionTags(tgcomp.Checkbox(p.Main, "顯示解法標籤（可能暴雷）", &tgcomp.CheckboxConf{
		Base:    tgframe.Base{ID: "show_solution_tags"},
		Default: showSolutionTags(),
	}))
	tgcomp.Caption(p.Main, "解法標籤會提示要用的演算法，預設收合；開啟後顯示在題目列表與題目頁，也能用來篩選")

	tgcomp.Subtitle(p.Main, "目前來源")
	s := openSet(p.Main, p.Context)
	if s == nil {
		return nil
	}
	if tgcomp.Button(p.Main, "檢查更新", &tgcomp.ButtonConf{ID: "source_refresh"}) {
		done := tgcomp.Spinner(p.Main, "檢查中…")
		err := s.Refresh(p.Context)
		done()
		if err != nil {
			tgcomp.MessageDanger(p.Main, "檢查更新失敗："+err.Error())
		}
	}
	if tgcomp.Button(p.Main, "全部下載（離線使用）",
		&tgcomp.ButtonConf{ID: "source_download"}) {
		st := tgcomp.Status(p.Main, "下載中…")
		err := s.DownloadAll(p.Context, func(done, total int) {
			st.Update(fmt.Sprintf("下載中 %d/%d", done, total))
		})
		if err != nil {
			st.Error("下載失敗")
			tgcomp.MessageDanger(p.Main, err.Error())
		} else {
			st.Complete("下載完成")
		}
	}

	es := s.Entries()
	cached := 0
	for _, e := range es {
		if e.Cached {
			cached++
		}
	}
	tgcomp.Text(p.Main, s.URL)
	tgcomp.Caption(p.Main, fmt.Sprintf("commit %s，上次檢查 %s，共 %d 題，%d 題可離線",
		s.Commit()[:7], s.Checked().Format("2006-01-02 15:04"), len(es), cached))
	return nil
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

const defaultSource = "https://github.com/mudream4869/offline-judge/tree/main/problems"

var (
	setMu      sync.Mutex
	curSet     *source.Set
	ghClient   = source.NewClient()
	probsStore source.Store
)

func sourceURL() string {
	return memo.getText("source", defaultSource)
}

// openSet returns the Set of the current source, loading its list on first
// use. On failure it shows why in c and returns nil.
func openSet(c *tgframe.Container, ctx context.Context) *source.Set {
	setMu.Lock()
	defer setMu.Unlock()
	url := sourceURL()
	if curSet == nil || curSet.URL != url {
		if probsStore == nil {
			probsStore = newProblemStore()
		}
		s, err := source.New(url, ghClient, probsStore)
		if err != nil {
			tgcomp.MessageDanger(c, "題目來源有誤："+err.Error())
			tgcomp.Link(c, "前往設定", "#/settings")
			return nil
		}
		curSet = s
	}
	s := curSet
	if s.Commit() != "" {
		return s
	}

	done := tgcomp.Spinner(c, "載入題目列表…")
	cached, err := s.Open(ctx)
	done()
	if err != nil {
		tgcomp.MessageDanger(c, "無法載入題目列表："+err.Error())
		tgcomp.Button(c, "重試")
		return nil
	}
	if cached {
		// Show the stored list now; a newer one shows on a later run.
		go func() {
			if err := s.Refresh(context.Background()); err != nil {
				log.Printf("檢查題目更新失敗：%v", err)
			}
		}()
	}
	return s
}
