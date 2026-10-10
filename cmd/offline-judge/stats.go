//go:build js && wasm

package main

import (
	"fmt"
	"time"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/stats"
)

// statDays is how many days the activity chart shows.
const statDays = 30

// Stats sums up practice from the submissions in this browser.
func Stats(p *tgframe.Params) error {
	tgcomp.Title(p.Main, "統計")
	subs := memo.allSubmissions()
	if len(subs) == 0 {
		tgcomp.Caption(p.Main, "還沒有提交紀錄")
		tgcomp.PageLink(p.Main, "前往題目列表", "problems", nil)
		return nil
	}
	ct := openCatalog(p.Main, p.Context)
	showSol := showSolutionTags()
	var ps []stats.Problem
	for i := range ct.entries {
		e := &ct.entries[i]
		if e.Unsupported == "" {
			ps = append(ps, stats.Problem{ID: e.ID, Tags: entryTags(e, showSol)})
		}
	}
	st := stats.Compute(subs, ps, time.Now(), statDays)
	tgcomp.Caption(p.Main, "依這個瀏覽器裡的提交紀錄計算")

	cols := tgcomp.Column(p.Main, 4, &tgcomp.ColumnConf{ID: "stats_tiles"})
	tgcomp.Metric(cols[0], "已通過", fmt.Sprintf("%d / %d 題", st.Listed, st.Problems))
	tgcomp.Metric(cols[1], "提交", fmt.Sprintf("%d 次", st.Submissions))
	tgcomp.Metric(cols[2], "AC 率", percent(st.AC, st.Submissions))
	tgcomp.Metric(cols[3], "連續練習", fmt.Sprintf("%d 天", st.Streak))
	tgcomp.Caption(p.Main, fmt.Sprintf("共練習 %d 天；%d 題提交過但還沒通過", st.ActiveDays, st.Tried))

	tgcomp.Subtitle(p.Main, fmt.Sprintf("最近 %d 天", statDays))
	labels := make([]string, len(st.Days))
	ac := make([]float64, len(st.Days))
	other := make([]float64, len(st.Days))
	for i, d := range st.Days {
		labels[i] = d.Date.Format("01/02")
		ac[i], other[i] = float64(d.AC), float64(d.Other)
	}
	tgcomp.BarChart(p.Main, labels, []tgcomp.ChartSeries{
		{Name: "AC", Values: ac},
		{Name: "未通過", Values: other},
	}, &tgcomp.ChartConf{
		Base:    tgframe.Base{ID: "stats_days"},
		Stacked: true,
		Height:  "260px",
		YLabel:  "提交次數",
	})

	if len(st.Tags) > 0 {
		tgcomp.Subtitle(p.Main, "依標籤")
		var rows [][]tgcomp.Cell
		for _, t := range st.Tags {
			rows = append(rows, []tgcomp.Cell{tgcomp.TextCell(t.Name), tgcomp.NumberCell(float64(t.Solved)),
				tgcomp.NumberCell(float64(t.Total)), ratioCell(t.Solved, t.Total)})
		}
		numberTable(p.Main, "stats_tags", []string{"標籤", "已通過", "題數", "完成度"}, rows)
	}

	tgcomp.Subtitle(p.Main, "依語言")
	var rows [][]tgcomp.Cell
	for _, l := range st.Langs {
		rows = append(rows, []tgcomp.Cell{tgcomp.TextCell(langByID(l.ID).name),
			tgcomp.NumberCell(float64(l.Submissions)), tgcomp.NumberCell(float64(l.AC)),
			ratioCell(l.AC, l.Submissions), tgcomp.NumberCell(float64(l.Solved))})
	}
	numberTable(p.Main, "stats_langs", []string{"語言", "提交", "AC", "AC 率", "通過題數"}, rows)

	tgcomp.Subtitle(p.Main, "依結果")
	rows = nil
	for _, v := range st.Verdicts {
		rows = append(rows, []tgcomp.Cell{tgcomp.TextCell(string(v.Verdict) + " " + verdictName(v.Verdict)),
			tgcomp.NumberCell(float64(v.Count)), ratioCell(v.Count, st.Submissions)})
	}
	numberTable(p.Main, "stats_verdicts", []string{"結果", "次數", "比例"}, rows)
	return nil
}

// numberTable is a sortable table whose columns but the first are numbers.
func numberTable(c *tgframe.Container, id string, head []string, rows [][]tgcomp.Cell) {
	cols := make([]tgcomp.DataFrameColumnConf, len(head))
	for i := 1; i < len(cols); i++ {
		cols[i].Type = tgcomp.ColumnTypeNumber
	}
	tgcomp.DataFrameCells(c, head, rows, (&tgcomp.DataFrameConf{
		Base:       tgframe.Base{ID: id},
		ColumnConf: cols,
	}).SetSearchable(false))
}

// ratioCell is n/total as a percentage, sorted by its value.
func ratioCell(n, total int) tgcomp.Cell {
	if total == 0 {
		return tgcomp.NumberCell(0).WithDisplay("-")
	}
	return tgcomp.NumberCell(float64(n) / float64(total)).WithDisplay(percent(n, total))
}

func percent(n, total int) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(n)/float64(total))
}
