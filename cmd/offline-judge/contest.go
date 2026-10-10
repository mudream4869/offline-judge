//go:build js && wasm

package main

import (
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgcomp/tcutil"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/contest"
)

// Contests runs mock contests: pick problems and a duration, then solve
// against the clock. Kept in this browser, like drafts.
func Contests(p *tgframe.Params) error {
	tgcomp.Title(p.Main, "模擬賽")
	startTicker()
	cs := loadContests()
	now := time.Now()

	// Clicks first, so everything below sees the change.
	cf := newConfirm(p, "contest")
	done := cf.confirmed()
	for i := range cs {
		c := &cs[i]
		if c.Running(now) && done == "end:"+contestID(c) {
			c.End = now
			saveContests(cs)
		}
		if c.Running(now) && tgcomp.ButtonClicked(p.Main, "提前結束", endConf(c)) {
			cf.ask("end:"+contestID(c), "提前結束「"+c.Title+"」？結束後不能再繼續作答。", "結束")
		}
	}
	for i := len(cs) - 1; i >= 0; i-- {
		if done == "del:"+contestID(&cs[i]) {
			cs = slices.Delete(cs, i, i+1)
			saveContests(cs)
			continue
		}
		if tgcomp.ButtonClicked(p.Main, delLabel, contestDelConf(&cs[i])) {
			cf.ask("del:"+contestID(&cs[i]), "刪除模擬賽「"+cs[i].Title+"」？刪除後無法復原。", "刪除")
		}
	}
	cf.draw()

	ct := openCatalog(p.Main, p.Context)
	if cur := runningContest(cs, now); cur != nil {
		showRunning(p, ct, cur, now)
	} else {
		newContest(p, ct, cs, now)
	}

	var past []*contest.Contest
	for i := len(cs) - 1; i >= 0; i-- {
		if !cs[i].Running(now) {
			past = append(past, &cs[i])
		}
	}
	if len(past) == 0 {
		return nil
	}
	tgcomp.Subtitle(p.Main, "過去的模擬賽")
	for _, c := range past {
		st := contest.Score(c, memo.allSubmissions())
		title := fmt.Sprintf("%s  %s  解 %d / %d 題，罰時 %d 分", c.Title, c.Start.Format("2006-01-02 15:04"),
			st.Solved, len(c.Problems), int(st.Penalty.Minutes()))
		box := tgcomp.Expand(p.Main, title, false, &tgcomp.ExpandConf{ID: "contest_" + contestID(c)})
		tgcomp.Caption(box, "時長 "+fmtClock(c.End.Sub(c.Start)))
		scoreboard(p, box, ct, c, &st)
		tgcomp.Button(box, delLabel, contestDelConf(c))
	}
	return nil
}

func showRunning(p *tgframe.Params, ct *catalog, c *contest.Contest, now time.Time) {
	st := contest.Score(c, memo.allSubmissions())
	tgcomp.Subtitle(p.Main, c.Title)
	tgcomp.Caption(p.Main, fmt.Sprintf("%s 開始，%s 結束；進行中的題目不顯示提示與解法標籤",
		c.Start.Format("15:04"), c.End.Format("15:04")))
	left, solved, penalty := tgcomp.Column3(p.Main, &tgcomp.ColumnConf{ID: "contest_metrics"})
	tgcomp.Metric(left, "剩餘時間", fmtClock(c.End.Sub(now)))
	tgcomp.Metric(solved, "解題數", fmt.Sprintf("%d / %d", st.Solved, len(c.Problems)))
	tgcomp.Metric(penalty, "罰時", fmt.Sprintf("%d 分", int(st.Penalty.Minutes())))
	scoreboard(p, p.Main, ct, c, &st)
	tgcomp.Button(p.Main, "提前結束", endConf(c))
}

// scoreboard lists c's problems with how each went; picking one opens it.
func scoreboard(p *tgframe.Params, box *tgframe.Container, ct *catalog, c *contest.Contest,
	st *contest.Standing) {

	titles := map[string]string{}
	for _, e := range ct.entries {
		titles[e.ID] = e.Title
	}
	scored := false // some problem has subtasks
	for _, r := range st.Results {
		scored = scored || r.Max > 0
	}
	head := []string{"題", "題目", "結果", "錯誤次數"}
	if scored {
		head = append(head, "子任務分數")
	}
	var rows [][]tgcomp.Cell
	for i, key := range c.Problems {
		r := st.Results[i]
		title := titles[key]
		if title == "" {
			title = key + "（來源中找不到）"
		}
		res := "-"
		switch {
		case r.Solved:
			res = "✓ " + fmtClock(r.At)
		case r.Wrong > 0:
			res = "✗"
		}
		row := []tgcomp.Cell{tgcomp.TextCell(contest.Label(i)), tgcomp.TextCell(title),
			tgcomp.TextCell(res), tgcomp.NumberCell(float64(r.Wrong))}
		if scored {
			score := ""
			if r.Max > 0 {
				score = fmt.Sprintf("%g / %g", r.Best, r.Max)
			}
			row = append(row, tgcomp.TextCell(score))
		}
		rows = append(rows, row)
	}
	cols := make([]tgcomp.DataFrameColumnConf, len(head))
	cols[3].Type = tgcomp.ColumnTypeNumber
	sel := tgcomp.DataFrameCells(box, head, rows,
		(&tgcomp.DataFrameConf{
			Base:       tgframe.Base{ID: "scoreboard_" + contestID(c)},
			ColumnConf: cols,
			Selection:  tgcomp.SelectionModeSingle,
			RowKeys:    c.Problems,
		}).SetSearchable(false))
	if len(sel) == 1 {
		p.Navigate("problems", pickQuery(nil, c.Problems[sel[0]]))
	}
}

// newContest draws the form that starts a contest.
func newContest(p *tgframe.Params, ct *catalog, cs []contest.Contest, now time.Time) {
	tgcomp.Subtitle(p.Main, "開始新的模擬賽")
	tgcomp.Caption(p.Main, "選幾題、設定時間後開始；比賽期間的提交依 ICPC 規則計分："+
		"罰時是每題解出的時間加上之前每次錯誤 20 分鐘（CE 不算）")
	var keys, names []string
	for _, e := range ct.entries {
		if e.Unsupported == "" {
			keys = append(keys, e.ID)
			names = append(names, e.Title+"（"+e.ID+"）")
		}
	}
	title := tgcomp.Textbox(p.Main, "名稱", &tgcomp.TextboxConf{
		Base:    tgframe.Base{ID: "contest_title"},
		Default: fmt.Sprintf("模擬賽 %d", len(cs)+1),
	})
	picked := tgcomp.MultiSelect(p.Main, "題目", names, &tgcomp.MultiSelectConf{
		Base:        tgframe.Base{ID: "contest_problems"},
		Placeholder: "選擇題目",
	})
	lo, hi := 1, 24*60
	mins, _ := tgcomp.Number(p.Main, "時間（分鐘）", &tgcomp.NumberConf[int]{
		Base:    tgframe.Base{ID: "contest_minutes"},
		Default: 120,
		Min:     &lo,
		Max:     &hi,
	})
	if !tgcomp.Button(p.Main, "開始", &tgcomp.ButtonConf{ID: "contest_start"}) {
		return
	}
	if len(picked) == 0 {
		tgcomp.MessageWarning(p.Main, "請至少選一題")
		return
	}
	if mins < lo {
		tgcomp.MessageWarning(p.Main, "時間至少 1 分鐘")
		return
	}
	if title == "" {
		title = "模擬賽"
	}
	c := contest.Contest{ID: now.UnixMilli(), Title: title, Start: now,
		End: now.Add(time.Duration(mins) * time.Minute)}
	for _, i := range picked {
		c.Problems = append(c.Problems, keys[i])
	}
	saveContests(append(cs, c))
	app.RerunPage("contest")
}

func loadContests() []contest.Contest {
	return contest.Parse(memo.getText("contests", ""))
}

func saveContests(cs []contest.Contest) {
	memo.setText("contests", contest.Marshal(cs))
}

// runningContest returns the contest on at now, if any.
func runningContest(cs []contest.Contest, now time.Time) *contest.Contest {
	for i := range cs {
		if cs[i].Running(now) {
			return &cs[i]
		}
	}
	return nil
}

// contestOf returns the running contest with problem key, if any.
func contestOf(key string) *contest.Contest {
	c := runningContest(loadContests(), time.Now())
	if c == nil || !c.Has(key) {
		return nil
	}
	return c
}

var tickerOnce sync.Once

// startTicker reruns the contest page every few seconds while a contest is
// on, and once more after it ends, to update the clock.
func startTicker() {
	tickerOnce.Do(func() {
		go func() {
			for range time.Tick(5 * time.Second) {
				now := time.Now()
				for _, c := range loadContests() {
					if c.Running(now) || now.Sub(c.End) < 10*time.Second && now.After(c.End) {
						app.RerunPage("contest")
						break
					}
				}
			}
		}()
	})
}

// fmtClock shows d as h:mm:ss.
func fmtClock(d time.Duration) string {
	d = max(d, 0).Truncate(time.Second)
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

func contestID(c *contest.Contest) string { return strconv.FormatInt(c.ID, 10) }

func endConf(c *contest.Contest) *tgcomp.ButtonConf {
	return &tgcomp.ButtonConf{ID: "contest_end_" + contestID(c), Color: tcutil.ColorDanger}
}

func contestDelConf(c *contest.Contest) *tgcomp.ButtonConf {
	return &tgcomp.ButtonConf{ID: "contest_del_" + contestID(c), Color: tcutil.ColorDanger}
}
