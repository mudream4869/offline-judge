//go:build js && wasm

package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/source"
	"github.com/mudream4869/offline-judge/problems"
)

// Submissions lists the submissions of every problem; picking a row opens it.
func Submissions(p *tgframe.Params) error {
	sub := pickedSubmission(p)
	if sub == nil {
		submissionList(p, memo.allSubmissions())
		return nil
	}
	if tgcomp.ButtonClicked(p.Main, delLabel, delConf(sub.ID)) {
		if err := memo.deleteSubmission(sub.ID); err != nil {
			tgcomp.MessageDanger(p.Main, "刪除失敗："+err.Error())
		} else {
			p.Navigate("submissions", listQuery(p.Query))
			return nil
		}
	}
	return showSubmission(p, sub)
}

// pickedSubmission returns the submission the query's id names; if it's
// gone, it says so and returns nil.
func pickedSubmission(p *tgframe.Params) *submission {
	v := p.Query.Get("id")
	if v == "" {
		return nil
	}
	if id, err := strconv.Atoi(v); err == nil {
		for _, s := range memo.allSubmissions() {
			if s.ID == id {
				return s
			}
		}
	}
	tgcomp.MessageWarning(p.Main, "找不到提交紀錄："+v)
	return nil
}

// submissionList draws the list; picking a row opens it. The search is kept
// in the query, so Back and the back link restore it.
func submissionList(p *tgframe.Params, subs []*submission) {
	tgcomp.Title(p.Main, "提交紀錄")
	if len(subs) == 0 {
		tgcomp.Caption(p.Main, "還沒有提交紀錄")
		return
	}
	search := tgcomp.Textbox(p.Sidebar, "搜尋提交紀錄", &tgcomp.TextboxConf{
		Base:        tgframe.Base{ID: "submission_search"},
		Placeholder: "編號、題目、語言、結果…",
		Default:     p.Query.Get("q"),
	})
	lq := url.Values{}
	if search != "" {
		lq.Set("q", search)
	}
	p.ReplaceQuery(lq)
	q := strings.ToLower(strings.TrimSpace(search))
	tgcomp.Caption(p.Main, fmt.Sprintf("共 %d 筆，存在這個瀏覽器裡；點一筆查看程式碼與結果", len(subs)))

	cur := currentEntries(p, p.Main)
	var ids []string
	var rows [][]tgcomp.Cell
	for _, s := range subs {
		t, tc := "-", tgcomp.MissingCell().WithDisplay("-")
		if s.Report.Verdict != judge.CE {
			mt := maxTime(&s.Report)
			t, tc = fmtTime(mt), tgcomp.NumberCell(float64(mt.Milliseconds())).WithDisplay(fmtTime(mt))
		}
		title, ver := s.Problem, s.Version
		if e, ok := cur[s.Problem]; ok {
			title = e.Title
			if outdated(s, e.Version) {
				ver += "（舊版）"
			}
		}
		row := []string{submissionNumber(s.ID), s.At.Format("2006-01-02 15:04:05"), title, ver,
			langByID(s.Lang).name, resultText(&s.Report), t}
		if q != "" && !strings.Contains(strings.ToLower(strings.Join(row, "\x00")+"\x00"+s.Problem), q) {
			continue
		}
		ids = append(ids, strconv.Itoa(s.ID))
		rows = append(rows, []tgcomp.Cell{
			tgcomp.NumberCell(float64(s.ID)).WithDisplay(row[0]),
			tgcomp.TimeCell(s.At).WithDisplay(row[1]),
			tgcomp.TextCell(row[2]), tgcomp.TextCell(row[3]), tgcomp.TextCell(row[4]),
			tgcomp.TextCell(row[5]), tc,
		})
	}
	if len(rows) == 0 {
		tgcomp.Caption(p.Main, "沒有符合搜尋的提交紀錄")
		return
	}
	cols := make([]tgcomp.DataFrameColumnConf, 7)
	cols[0].Type = tgcomp.ColumnTypeNumber
	cols[1].Type = tgcomp.ColumnTypeDatetime
	cols[6].Type = tgcomp.ColumnTypeNumber
	sel := tgcomp.DataFrameCells(p.Main, []string{"編號", "時間", "題目", "版本", "語言", "結果", "最長耗時"}, rows,
		(&tgcomp.DataFrameConf{
			Base:       tgframe.Base{ID: "submission_list"},
			PageSize:   50,
			ColumnConf: cols,
			Selection:  tgcomp.SelectionModeSingle,
			RowKeys:    ids,
		}).SetSearchable(false))
	if len(sel) == 1 {
		p.Navigate("submissions", pickQuery(lq, ids[sel[0]]))
	}
}

// currentEntries maps problem keys to entries of the sources that load.
func currentEntries(p *tgframe.Params, c *tgframe.Container) map[string]source.Entry {
	out := map[string]source.Entry{}
	slot := tgcomp.Empty(c)
	var ct *catalog
	slot.With(func(c *tgframe.Container) { ct = openCatalog(c, p.Context) })
	// The problem page says why a source fails; here keys will do.
	slot.Clear()
	for _, e := range ct.entries {
		out[e.ID] = e
	}
	return out
}

func showSubmission(p *tgframe.Params, sub *submission) error {
	tgcomp.PageLink(p.Main, "← 返回提交紀錄", "submissions", listQuery(p.Query))

	// Load the problem to show the failed test; it may be gone from the source.
	var pr *problems.Problem
	title, found := sub.Problem, false
	ct := openCatalog(p.Main, p.Context)
	for _, e := range ct.entries {
		if e.ID != sub.Problem {
			continue
		}
		title, found = e.Title, true
		done := func() {}
		if !e.Cached {
			done = tgcomp.Spinner(p.Main, "下載題目中…")
		}
		var err error
		pr, err = ct.problem(p.Context, e.ID)
		done()
		if err != nil {
			tgcomp.MessageWarning(p.Main, "無法載入題目："+err.Error())
		}
	}

	lg := langByID(sub.Lang)
	tgcomp.Subtitle(p.Main, fmt.Sprintf("%s  %s", submissionNumber(sub.ID), title))
	info := fmt.Sprintf("%s，%s", sub.At.Format("2006-01-02 15:04:05"), lg.name)
	if sub.Version != "" {
		info += "，題目版本 " + sub.Version
	}
	tgcomp.Caption(p.Main, info)
	warnUnsavedSubmission(p.Main, sub)
	if found {
		tgcomp.PageLink(p.Main, "前往題目", "problems", url.Values{"id": {sub.Problem}})
	}
	if pr != nil && outdated(sub, pr.Version) {
		tgcomp.MessageWarning(p.Main, "題目已更新（目前版本 "+pr.Version+"），下方測資可能與當時不同")
	}
	tgcomp.Code(p.Main, sub.Code, &tgcomp.CodeConf{Language: lg.hl})
	showReport(p.Main, pr, &sub.Report, fmt.Sprintf("subpage_%d", sub.ID))
	rejudgeButton(p.Context, p.Main, pr, sub, fmt.Sprintf("subpage_%d", sub.ID))
	tgcomp.Button(p.Main, delLabel, delConf(sub.ID))
	return nil
}

// outdated reports whether s was judged against another version of its
// problem; unknown for submissions made before versions.
func outdated(s *submission, cur string) bool {
	return s.Version != "" && s.Version != cur
}

func submissionNumber(id int) string {
	if id < 0 {
		return fmt.Sprintf("暫存 #%d", -id)
	}
	return fmt.Sprintf("#%d", id)
}

func warnUnsavedSubmission(c *tgframe.Container, sub *submission) {
	if sub.ID < 0 {
		tgcomp.MessageWarning(c, "此紀錄尚未保存，重新整理後會消失")
	}
}
