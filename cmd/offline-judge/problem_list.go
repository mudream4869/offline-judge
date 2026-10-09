//go:build js && wasm

package main

import (
	"fmt"
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

	// A linked problem is picked once, so "back" still reaches the list.
	if id := p.Query.Get("id"); id != "" {
		if seen, _ := p.State.Get[string](linkedKey); seen != id {
			p.State.Set(linkedKey, id)
			p.State.Set(pickedKey, id)
		}
	}
	// Before the list, so the cleared pick takes effect this run.
	if tgcomp.ButtonClicked(p.Main, backLabel, backConf) {
		p.State.Delete(pickedKey)
	}
	e := problemList(p, es)
	if e == nil {
		return nil
	}

	tgcomp.Button(p.Main, backLabel, backConf)
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

const (
	pickedKey = "picked_problem"
	linkedKey = "linked_problem"
	backLabel = "← 返回題目列表"
)

var backConf = &tgcomp.ButtonConf{ID: "problem_back"}

// problemList returns the picked entry, or draws the list if there is none.
func problemList(p *tgframe.Params, es []source.Entry) *source.Entry {
	// A pick gone from a newer list falls back to the list.
	if id, ok := p.State.Get[string](pickedKey); ok {
		for i := range es {
			if es[i].ID == id {
				return &es[i]
			}
		}
	}

	// Filters sit in the sidebar; both slots clear once a problem is picked.
	side := tgcomp.Empty(p.Sidebar)
	showSol := showSolutionTags()
	var query string
	var want []string
	filter := 0
	side.With(func(c *tgframe.Container) {
		query = strings.ToLower(strings.TrimSpace(tgcomp.Textbox(c, "搜尋", &tgcomp.TextboxConf{
			Base:        tgframe.Base{ID: "problem_search"},
			Placeholder: "編號或題目",
		})))
		if tags := allTags(es, showSol); len(tags) > 0 {
			for _, i := range tgcomp.MultiSelect(c, "標籤", tags, &tgcomp.MultiSelectConf{
				Base:        tgframe.Base{ID: "problem_tags"},
				Placeholder: "全部",
			}) {
				want = append(want, tags[i])
			}
		}
		if i := tgcomp.Select(c, "狀態", statusFilters, (&tgcomp.SelectConf{
			Base: tgframe.Base{ID: "problem_status"},
		}).SetDefault(0)); i != nil {
			filter = *i
		}
	})
	status := solveStatus(memo.allSubmissions())

	slot := tgcomp.Empty(p.Main)
	var shown []*source.Entry
	var sel []int
	slot.With(func(c *tgframe.Container) {
		tgcomp.Title(c, "題目列表")
		tgcomp.Caption(c, "點一題開始作答")

		var ids []string
		var rows [][]string
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
			shown = append(shown, e)
			ids = append(ids, e.ID)
			rows = append(rows, []string{st, num, e.Title, strings.Join(tags, "、"),
				fmtLimits(e.TimeLimit, e.TimeLimits), e.Version, off})
		}
		if len(rows) == 0 {
			tgcomp.MessageInfo(c, "沒有符合的題目")
			return
		}
		sel = tgcomp.DataFrame(c, []string{"狀態", "編號", "題目", "標籤", "時間限制", "版本", "可離線"}, rows,
			(&tgcomp.DataFrameConf{
				Base:      tgframe.Base{ID: "problem_list"},
				PageSize:  100,
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
	p.State.Set(pickedKey, shown[sel[0]].ID)
	return shown[sel[0]]
}

// statusFilters are the choices of the list's status filter.
var statusFilters = []string{"全部", "已通過", "尚未通過（含未提交）"}

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
