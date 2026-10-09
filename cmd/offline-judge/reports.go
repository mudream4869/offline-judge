//go:build js && wasm

package main

import (
	"fmt"
	"time"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgcomp/tcutil"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/problems"
)

// Inputs/outputs longer than this are cut in the result view.
const showLimit = 2000

const delLabel = "刪除這筆紀錄"

func delConf(id int) *tgcomp.ButtonConf {
	return &tgcomp.ButtonConf{ID: fmt.Sprintf("del_%d", id), Color: tcutil.ColorDanger}
}

// slim keeps only what showReport shows, so stored reports stay small.
func slim(rep judge.Report) judge.Report {
	rep.CompileError = cut(rep.CompileError)
	cases := make([]judge.CaseResult, len(rep.Cases))
	shown := false
	for i, cr := range rep.Cases {
		if cr.Verdict == judge.AC || shown {
			cr.Stdout, cr.Stderr = "", ""
		} else {
			shown = true
			cr.Stdout, cr.Stderr, cr.Message = cut(cr.Stdout), cut(cr.Stderr), cut(cr.Message)
		}
		cases[i] = cr
	}
	rep.Cases = cases
	return rep
}

func maxTime(rep *judge.Report) time.Duration {
	var t time.Duration
	for _, cr := range rep.Cases {
		t = max(t, cr.Time)
	}
	return t
}

// showReport draws rep; id keeps its components unique on the page.
func showReport(c *tgframe.Container, pr *problems.Problem, rep *judge.Report, id string) {
	score := ""
	if rep.MaxScore > 0 {
		score = "（" + scoreText(rep) + "）"
	}
	switch {
	case rep.Verdict == judge.AC:
		tgcomp.MessageSuccess(c, "AC：全部通過"+score)
	case rep.Score > 0:
		tgcomp.MessageWarning(c, string(rep.Verdict)+"："+verdictName(rep.Verdict)+score)
	default:
		tgcomp.MessageDanger(c, string(rep.Verdict)+"："+verdictName(rep.Verdict)+score)
	}
	if rep.Verdict == judge.CE {
		tgcomp.Code(c, cut(rep.CompileError), &tgcomp.CodeConf{Language: "text"})
		return
	}

	if len(rep.Subtasks) > 0 {
		rows := make([][]string, len(rep.Subtasks))
		for i, st := range rep.Subtasks {
			rows[i] = []string{fmt.Sprint(i + 1), string(st.Verdict), fmt.Sprintf("%g / %g", st.Score, st.Max)}
		}
		tgcomp.Table(c, []string{"子任務", "結果", "分數"}, rows)
	}

	rows := make([][]string, len(rep.Cases))
	for i, cr := range rep.Cases {
		t := fmtTime(cr.Time)
		if cr.Verdict == judge.Skip {
			t = "-"
		}
		rows[i] = []string{cr.Name, string(cr.Verdict), t}
	}
	tgcomp.Table(c, []string{"測資", "結果", "時間"}, rows)

	// Tests aren't secret, so show the first failure in full.
	for i, cr := range rep.Cases {
		if cr.Verdict == judge.AC {
			continue
		}
		// The tests may have changed since, or the problem isn't loaded.
		if pr == nil || i >= len(pr.Cases) || pr.Cases[i].Name != cr.Name {
			break
		}
		tc := pr.Cases[i]
		box := tgcomp.Expand(c, "第一筆失敗："+cr.Name, true,
			&tgcomp.ExpandConf{ID: "fail_" + id})
		interactive := pr.Interactor != ""
		if interactive {
			tgcomp.Text(box, "互動程式的輸入")
		} else {
			tgcomp.Text(box, "輸入")
		}
		tgcomp.Code(box, cut(tc.Input), &tgcomp.CodeConf{Language: "text"})
		switch {
		case interactive:
		case pr.Checker != "":
			tgcomp.Text(box, "參考輸出（答案不唯一）")
			tgcomp.Code(box, cut(tc.Output), &tgcomp.CodeConf{Language: "text"})
		default:
			tgcomp.Text(box, "預期輸出")
			tgcomp.Code(box, cut(tc.Output), &tgcomp.CodeConf{Language: "text"})
		}
		if cr.Verdict != judge.TLE {
			tgcomp.Text(box, stdoutLabel(pr))
			tgcomp.Code(box, cut(cr.Stdout), &tgcomp.CodeConf{Language: "text"})
		}
		if cr.Message != "" {
			switch {
			case interactive:
				tgcomp.Text(box, "互動程式訊息")
			case pr.Checker != "":
				tgcomp.Text(box, "checker 訊息")
			default:
				tgcomp.Text(box, "比對結果")
			}
			tgcomp.Code(box, cr.Message, &tgcomp.CodeConf{Language: "text"})
		}
		if cr.Stderr != "" {
			tgcomp.Text(box, "stderr")
			tgcomp.Code(box, cut(cr.Stderr), &tgcomp.CodeConf{Language: "text"})
		}
		break
	}
}

func verdictName(v judge.Verdict) string {
	switch v {
	case judge.WA:
		return "答案錯誤"
	case judge.TLE:
		return "超過時間限制"
	case judge.RE:
		return "執行錯誤"
	case judge.CE:
		return "編譯錯誤"
	case judge.OLE:
		return fmt.Sprintf("輸出超過 %d MB", outputLimit>>20)
	}
	return ""
}

// scoreText is rep's score, e.g. "40 / 100 分".
func scoreText(rep *judge.Report) string {
	return fmt.Sprintf("%g / %g 分", rep.Score, rep.MaxScore)
}

// resultText is rep's verdict, with the score if the problem has subtasks.
func resultText(rep *judge.Report) string {
	if rep.MaxScore == 0 {
		return string(rep.Verdict)
	}
	return string(rep.Verdict) + "（" + scoreText(rep) + "）"
}

func fmtTime(d time.Duration) string {
	return fmt.Sprintf("%d ms", d.Milliseconds())
}

func cut(s string) string {
	if len(s) <= showLimit {
		return s
	}
	return s[:showLimit] + "\n…（已截斷）"
}
