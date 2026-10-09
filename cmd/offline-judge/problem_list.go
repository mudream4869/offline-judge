//go:build js && wasm

package main

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/source"
)

// Problems lists the problems; picking a row opens it.
func Problems(p *tgframe.Params) error {
	s := openSet(p.Main, p.Context)
	if s == nil {
		return nil
	}
	es := s.Entries()
	if len(es) == 0 {
		tgcomp.MessageWarning(p.Main, "這個來源沒有題目")
		return nil
	}

	e := pickedEntry(p, es)
	if e == nil {
		problemList(p, es)
		return nil
	}

	tgcomp.PageLink(p.Main, "← 返回題目列表", "problems", listQuery(p.Query))
	done := func() {}
	if !e.Cached {
		done = tgcomp.Spinner(p.Main, "下載題目中…")
	}
	pr, err := s.Problem(p.Context, e.ID)
	done()
	if err != nil {
		tgcomp.MessageDanger(p.Main, "無法載入題目："+err.Error())
		return nil
	}
	return showProblem(p, pr)
}

// pickedEntry returns the entry the query's id names; if it's gone from a
// newer list, it says so and returns nil.
func pickedEntry(p *tgframe.Params, es []source.Entry) *source.Entry {
	id := p.Query.Get("id")
	if id == "" {
		return nil
	}
	for i := range es {
		if es[i].ID == id {
			return &es[i]
		}
	}
	tgcomp.MessageWarning(p.Main, "找不到題目："+id)
	return nil
}

// listQuery is q without the picked id, so a list keeps its filters.
func listQuery(q url.Values) url.Values {
	q = maps.Clone(q)
	q.Del("id")
	return q
}

// pickQuery is the list's query q with id picked.
func pickQuery(q url.Values, id string) url.Values {
	q = maps.Clone(q)
	if q == nil {
		q = url.Values{}
	}
	q.Set("id", id)
	return q
}

// problemList draws the list; picking a row opens it. The filters are kept
// in the query, so Back and the back link restore them.
func problemList(p *tgframe.Params, es []source.Entry) {
	showSol := showSolutionTags()
	q := url.Values{}
	search := tgcomp.Textbox(p.Sidebar, "搜尋", &tgcomp.TextboxConf{
		Base:        tgframe.Base{ID: "problem_search"},
		Placeholder: "編號或題目",
		Default:     p.Query.Get("q"),
	})
	if search != "" {
		q.Set("q", search)
	}
	query := strings.ToLower(strings.TrimSpace(search))
	var want []string
	if tags := allTags(es, showSol); len(tags) > 0 {
		var def []int
		for _, t := range p.Query["tag"] {
			if i := slices.Index(tags, t); i >= 0 {
				def = append(def, i)
			}
		}
		for _, i := range tgcomp.MultiSelect(p.Sidebar, "標籤", tags, &tgcomp.MultiSelectConf{
			Base:        tgframe.Base{ID: "problem_tags"},
			Placeholder: "全部",
			Default:     def,
		}) {
			want = append(want, tags[i])
		}
		q["tag"] = want
	}
	filter := max(slices.Index(statusKeys, p.Query.Get("status")), 0)
	if i := tgcomp.Select(p.Sidebar, "狀態", statusFilters, (&tgcomp.SelectConf{
		Base: tgframe.Base{ID: "problem_status"},
	}).SetDefault(filter)); i != nil {
		filter = *i
	}
	if filter > 0 {
		q.Set("status", statusKeys[filter])
	}
	p.ReplaceQuery(q)
	status := solveStatus(memo.allSubmissions())

	tgcomp.Title(p.Main, "題目列表")
	tgcomp.Caption(p.Main, "點一題開始作答")
	var ids []string
	var rows [][]tgcomp.Cell
	for i := range es {
		e := &es[i]
		num := problemNumber(e.ID)
		tags := entryTags(e, showSol)
		st := status[e.ID]
		if !hasTags(tags, want) ||
			!strings.Contains(strings.ToLower(num+" "+e.Title), query) ||
			filter == 1 && st != solvedMark || filter == 2 && st == solvedMark {
			continue
		}
		off := ""
		if e.Cached {
			off = "✓"
		}
		ids = append(ids, e.ID)
		limit := tgcomp.NumberCell(float64(e.TimeLimit.Milliseconds())).
			WithDisplay(fmtLimits(e.TimeLimit, e.TimeLimits))
		rows = append(rows, []tgcomp.Cell{tgcomp.TextCell(st), tgcomp.TextCell(num),
			tgcomp.TextCell(e.Title), tgcomp.TextCell(strings.Join(tags, "、")), limit,
			tgcomp.TextCell(e.Version), tgcomp.TextCell(off)})
	}
	if len(rows) == 0 {
		tgcomp.MessageInfo(p.Main, "沒有符合的題目")
		return
	}
	cols := make([]tgcomp.DataFrameColumnConf, 7)
	cols[4].Type = tgcomp.ColumnTypeNumber // sorted by the default limit
	sel := tgcomp.DataFrameCells(p.Main, []string{"狀態", "編號", "題目", "標籤", "時間限制", "版本", "可離線"}, rows,
		(&tgcomp.DataFrameConf{
			Base:       tgframe.Base{ID: "problem_list"},
			PageSize:   100,
			ColumnConf: cols,
			Selection:  tgcomp.SelectionModeSingle,
			RowKeys:    ids,
		}).SetSearchable(false))
	if len(sel) == 1 {
		p.Navigate("problems", pickQuery(q, ids[sel[0]]))
	}
}

// statusFilters are the choices of the list's status filter.
var statusFilters = []string{"全部", "已通過", "尚未通過（含未提交）"}

// statusKeys name statusFilters in the query.
var statusKeys = []string{"", "solved", "unsolved"}

// Marks of the list's status column.
const (
	solvedMark = "已通過" // some submission is AC
	triedMark  = "未通過" // submitted, never AC
)

// solveStatus maps each submitted problem to solvedMark, or triedMark
// with the best partial score if any.
func solveStatus(subs []*submission) map[string]string {
	st := map[string]string{}
	best := map[string]float64{}
	for _, s := range subs {
		if s.Report.Verdict == judge.AC {
			st[s.Problem] = solvedMark
		} else if st[s.Problem] == "" {
			st[s.Problem] = triedMark
		}
		best[s.Problem] = max(best[s.Problem], s.Report.Score)
	}
	for p, v := range st {
		if v == triedMark && best[p] > 0 {
			st[p] = fmt.Sprintf("%s（最高 %g 分）", triedMark, best[p])
		}
	}
	return st
}

// problemNumber returns the leading digits of id ("0001-a-plus-b" → "0001"), or id if none.
func problemNumber(id string) string {
	n := 0
	for n < len(id) && id[n] >= '0' && id[n] <= '9' {
		n++
	}
	if n == 0 {
		return id
	}
	return id[:n]
}

// allTags returns the tags of es, sorted, without duplicates.
func allTags(es []source.Entry, showSol bool) []string {
	var tags []string
	for i := range es {
		tags = append(tags, entryTags(&es[i], showSol)...)
	}
	slices.Sort(tags)
	return slices.Compact(tags)
}

// entryTags returns the tags of e, with its solution tags if showSol.
func entryTags(e *source.Entry, showSol bool) []string {
	if !showSol {
		return e.Tags
	}
	return append(slices.Clip(e.Tags), e.SolutionTags...)
}

// hasTags reports whether tags contains every tag in want.
func hasTags(tags, want []string) bool {
	for _, w := range want {
		if !slices.Contains(tags, w) {
			return false
		}
	}
	return true
}

// fmtLimits shows limit, then each language that overrides it, e.g. "2000 ms（C++ 500 ms）".
func fmtLimits(limit time.Duration, byLang map[string]time.Duration) string {
	var over []string
	for _, l := range langs {
		if t, ok := byLang[l.id]; ok {
			over = append(over, l.name+" "+fmtTime(t))
		}
	}
	if len(over) == 0 {
		return fmtTime(limit)
	}
	return fmtTime(limit) + "（" + strings.Join(over, "、") + "）"
}
