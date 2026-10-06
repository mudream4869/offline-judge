//go:build js && wasm

// Command offline-judge is a Python judge that runs entirely in the browser.
package main

import (
	"fmt"
	"log"
	"syscall/js"
	"time"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"
	"github.com/voilelab/toolgui/toolgui/tgwasm"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/problems"
)

const (
	defaultCode = "import sys\ninput = sys.stdin.readline\n\n"
	// Inputs/outputs longer than this are cut in the result view.
	showLimit = 2000
)

var (
	probs  []*problems.Problem
	runner *PyRunner
)

func Index(p *tgframe.Params) error {
	titles := make([]string, len(probs))
	for i, pr := range probs {
		titles[i] = pr.Title
	}
	sel := tgcomp.Select(p.Sidebar, "題目", titles, (&tgcomp.SelectConf{}).SetDefault(0))
	if sel == nil {
		tgcomp.Text(p.Main, "請選擇題目")
		return nil
	}
	pr := probs[*sel]

	if runner.Ready() {
		tgcomp.Caption(p.Sidebar, "Python 環境：就緒")
	} else {
		tgcomp.Caption(p.Sidebar, "Python 環境：載入中（首次約需數秒）")
	}

	tgcomp.Markdown(p.Main, pr.Statement)
	tgcomp.Caption(p.Main, fmt.Sprintf("時間限制：%d ms", pr.TimeLimit.Milliseconds()))

	for i, c := range pr.Samples() {
		tgcomp.Subtitle(p.Main, fmt.Sprintf("範例 %d", i+1))
		in, out := tgcomp.EqColumn2(p.Main, &tgcomp.ColumnConf{ID: "sample_" + c.Name})
		tgcomp.Text(in, "輸入")
		tgcomp.Code(in, c.Input, &tgcomp.CodeConf{Language: "text"})
		tgcomp.Text(out, "輸出")
		tgcomp.Code(out, c.Output, &tgcomp.CodeConf{Language: "text"})
	}

	tgcomp.Divider(p.Main)

	// One textarea per problem, so switching problems keeps the code.
	code := tgcomp.Textarea(p.Main, "程式碼（Python）", &tgcomp.TextareaConf{
		ID:      "code_" + pr.ID,
		Height:  16,
		Default: defaultCode,
	})

	submitTab, customTab := tgcomp.Tab2(p.Main, "提交", "自訂輸入")
	submitPanel(p, submitTab, pr, code)
	customPanel(p, customTab, pr, code)
	return nil
}

func submitPanel(p *tgframe.Params, c *tgframe.Container, pr *problems.Problem, code string) {
	key := "report_" + pr.ID
	if tgcomp.Button(c, "提交", &tgcomp.ButtonConf{ID: "submit_" + pr.ID}) {
		st := tgcomp.Status(c, "評測中…")
		rep, err := judge.Judge(p.Context, runner, code, pr.Cases, pr.TimeLimit,
			func(done int, cr judge.CaseResult) {
				st.Update(fmt.Sprintf("評測中 %d/%d", done, len(pr.Cases)))
				st.Write(fmt.Sprintf("%s：%s（%s）", cr.Name, cr.Verdict, fmtTime(cr.Time)))
			})
		if err != nil {
			st.Error("評測失敗")
			tgcomp.MessageDanger(c, err.Error())
			return
		}
		st.Complete("評測完成")
		p.State.Set(key, &rep)
	}

	rep, ok := p.State.Get[*judge.Report](key)
	if !ok || rep == nil {
		return
	}
	showReport(c, pr, rep)
}

func showReport(c *tgframe.Container, pr *problems.Problem, rep *judge.Report) {
	if rep.Verdict == judge.AC {
		tgcomp.MessageSuccess(c, "AC：全部通過")
	} else {
		tgcomp.MessageDanger(c, string(rep.Verdict)+"："+verdictName(rep.Verdict))
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
		tc := pr.Cases[i]
		box := tgcomp.Expand(c, "第一筆失敗："+cr.Name, true)
		tgcomp.Text(box, "輸入")
		tgcomp.Code(box, cut(tc.Input), &tgcomp.CodeConf{Language: "text"})
		tgcomp.Text(box, "預期輸出")
		tgcomp.Code(box, cut(tc.Output), &tgcomp.CodeConf{Language: "text"})
		if cr.Verdict != judge.TLE {
			tgcomp.Text(box, "你的輸出")
			tgcomp.Code(box, cut(cr.Stdout), &tgcomp.CodeConf{Language: "text"})
		}
		if cr.Stderr != "" {
			tgcomp.Text(box, "stderr")
			tgcomp.Code(box, cut(cr.Stderr), &tgcomp.CodeConf{Language: "text"})
		}
		break
	}
}

func customPanel(p *tgframe.Params, c *tgframe.Container, pr *problems.Problem, code string) {
	stdin := tgcomp.Textarea(c, "輸入", &tgcomp.TextareaConf{
		ID:      "stdin_" + pr.ID,
		Height:  6,
		Default: pr.Cases[0].Input,
	})
	if !tgcomp.Button(c, "執行", &tgcomp.ButtonConf{ID: "run_" + pr.ID}) {
		return
	}

	done := tgcomp.Spinner(c, "執行中…")
	res, err := runner.Run(p.Context, code, stdin, pr.TimeLimit)
	done()
	if err != nil {
		tgcomp.MessageDanger(c, err.Error())
		return
	}

	switch {
	case res.Status == judge.RunTimeout || res.Time > pr.TimeLimit:
		tgcomp.MessageWarning(c, "TLE："+fmtTime(res.Time))
	case res.Status == judge.RunError:
		tgcomp.MessageDanger(c, "RE："+fmtTime(res.Time))
	default:
		tgcomp.MessageInfo(c, "執行完成："+fmtTime(res.Time))
	}
	tgcomp.Text(c, "stdout")
	tgcomp.Code(c, cut(res.Stdout), &tgcomp.CodeConf{Language: "text"})
	if res.Stderr != "" {
		tgcomp.Text(c, "stderr")
		tgcomp.Code(c, cut(res.Stderr), &tgcomp.CodeConf{Language: "text"})
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
	}
	return ""
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

// workerURL resolves the Python worker against this (Go) worker's script,
// which toolgui-wasm puts in static/.
func workerURL() string {
	href := js.Global().Get("location").Get("href")
	return js.Global().Get("URL").New("../assets/pyworker.mjs", href).Get("href").String()
}

func main() {
	var err error
	probs, err = problems.All()
	if err != nil {
		log.Fatal(err)
	}
	runner = NewPyRunner(workerURL())

	app := tgframe.NewApp()
	app.AddPage("index", "Offline Judge", Index)
	app.SetHashPageNameMode(true)
	tgwasm.NewExecutor(app).Run()
}
