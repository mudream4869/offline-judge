//go:build js && wasm

package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/source"
	"github.com/mudream4869/offline-judge/problems"
)

const (
	pickedSubKey = "picked_submission"
	subQueryKey  = "submission_query"
	subBackLabel = "← 返回提交紀錄"
)

var subBackConf = &tgcomp.ButtonConf{ID: "submission_back"}

// Submissions lists the submissions of every problem; picking a row opens it.
func Submissions(p *tgframe.Params) error {
	// Before the list, so a cleared pick takes effect this run.
	if tgcomp.ButtonClicked(p.Main, subBackLabel, subBackConf) {
		p.State.Delete(pickedSubKey)
	}
	if sub := pickedSubmission(p); sub != nil && tgcomp.ButtonClicked(p.Main, delLabel, delConf(sub.ID)) {
		memo.deleteSubmission(sub.Problem, sub.ID)
		p.State.Delete(pickedSubKey)
	}

	subs := memo.allSubmissions()
	sub := pickedSubmission(p)
	if sub == nil {
		sub = submissionList(p, subs)
	}
	if sub == nil {
		return nil
	}
	return showSubmission(p, sub)
}

// pickedSubmission returns the picked submission, if it still exists.
func pickedSubmission(p *tgframe.Params) *submission {
	id, ok := p.State.GetNumber[int](pickedSubKey)
	if !ok {
		return nil
	}
	for _, s := range memo.allSubmissions() {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// submissionList returns the picked submission, or draws the list if there is none.
func submissionList(p *tgframe.Params, subs []*submission) *submission {
	// Search sits in the sidebar and is cleared with the list once a row is
	// picked; the query is kept apart so it comes back with the list.
	side := tgcomp.Empty(p.Sidebar)
	var q string
	if len(subs) > 0 {
		saved, _ := p.State.Get[string](subQueryKey)
		side.With(func(c *tgframe.Container) {
			q = tgcomp.Textbox(c, "搜尋提交紀錄", &tgcomp.TextboxConf{
				Base:        tgframe.Base{ID: "submission_search"},
				Placeholder: "編號、題目、語言、結果…",
				Default:     saved,
			})
		})
		p.State.Set(subQueryKey, q)
	}
	q = strings.ToLower(strings.TrimSpace(q))

	slot := tgcomp.Empty(p.Main)
	var sel []int
	slot.With(func(c *tgframe.Container) {
		tgcomp.Title(c, "提交紀錄")
		if len(subs) == 0 {
			tgcomp.Caption(c, "還沒有提交紀錄")
			return
		}
		tgcomp.Caption(c, fmt.Sprintf("共 %d 筆，存在這個瀏覽器裡；點一筆查看程式碼與結果", len(subs)))

		cur := currentEntries(p, c)
		var shown []*submission
		var ids []string
		var rows [][]string
		for _, s := range subs {
			t := "-"
			if s.Report.Verdict != judge.CE {
				t = fmtTime(maxTime(&s.Report))
			}
			title, ver := s.Problem, s.Version
			if e, ok := cur[s.Problem]; ok {
				title = e.Title
				if outdated(s, e.Version) {
					ver += "（舊版）"
				}
			}
			id := strconv.Itoa(s.ID)
			row := []string{"#" + id, s.At.Format("2006-01-02 15:04:05"), title, ver,
				langByID(s.Lang).name, string(s.Report.Verdict), t}
			if q != "" && !strings.Contains(strings.ToLower(strings.Join(row, "\x00")+"\x00"+s.Problem), q) {
				continue
			}
			shown = append(shown, s)
			ids = append(ids, id)
			rows = append(rows, row)
		}
		subs = shown
		if len(rows) == 0 {
			tgcomp.Caption(c, "沒有符合搜尋的提交紀錄")
			return
		}
		sel = tgcomp.DataFrame(c, []string{"編號", "時間", "題目", "版本", "語言", "結果", "最長耗時"}, rows,
			(&tgcomp.DataFrameConf{
				Base:      tgframe.Base{ID: "submission_list"},
				PageSize:  50,
				Selection: tgcomp.SelectionModeSingle,
				RowKeys:   ids,
			}).SetSortable(false).SetSearchable(false))
	})
	if len(sel) == 0 {
		return nil
	}
	// Clearing drops the list's pick too, so it comes back unpicked.
	slot.Clear()
	side.Clear()
	p.State.Set(pickedSubKey, subs[sel[0]].ID)
	return subs[sel[0]]
}

// currentEntries maps problem IDs to entries of the current source; empty if
// it can't be loaded.
func currentEntries(p *tgframe.Params, c *tgframe.Container) map[string]source.Entry {
	out := map[string]source.Entry{}
	slot := tgcomp.Empty(c)
	var s *source.Set
	slot.With(func(c *tgframe.Container) { s = openSet(c, p.Context) })
	if s == nil {
		// The problem page shows why; here IDs will do.
		slot.Clear()
		return out
	}
	for _, e := range s.Entries() {
		out[e.ID] = e
	}
	return out
}

func showSubmission(p *tgframe.Params, sub *submission) error {
	tgcomp.Button(p.Main, subBackLabel, subBackConf)

	// Load the problem to show the failed test; it may be gone from the source.
	var pr *problems.Problem
	title := sub.Problem
	if s := openSet(p.Main, p.Context); s != nil {
		for _, e := range s.Entries() {
			if e.ID != sub.Problem {
				continue
			}
			title = e.Title
			done := func() {}
			if !e.Cached {
				done = tgcomp.Spinner(p.Main, "下載題目中…")
			}
			var err error
			pr, err = s.Problem(p.Context, e.ID)
			done()
			if err != nil {
				tgcomp.MessageWarning(p.Main, "無法載入題目："+err.Error())
			}
		}
	}

	lg := langByID(sub.Lang)
	tgcomp.Subtitle(p.Main, fmt.Sprintf("#%d  %s", sub.ID, title))
	info := fmt.Sprintf("%s，%s", sub.At.Format("2006-01-02 15:04:05"), lg.name)
	if sub.Version != "" {
		info += "，題目版本 " + sub.Version
	}
	tgcomp.Caption(p.Main, info)
	if pr != nil && outdated(sub, pr.Version) {
		tgcomp.MessageWarning(p.Main, "題目已更新（目前版本 "+pr.Version+"），下方測資可能與當時不同")
	}
	tgcomp.Code(p.Main, sub.Code, &tgcomp.CodeConf{Language: lg.hl})
	showReport(p.Main, pr, &sub.Report, fmt.Sprintf("subpage_%d", sub.ID))
	tgcomp.Button(p.Main, delLabel, delConf(sub.ID))
	return nil
}

// outdated reports whether s was judged against another version of its
// problem; unknown for submissions made before versions.
func outdated(s *submission, cur string) bool {
	return s.Version != "" && s.Version != cur
}
