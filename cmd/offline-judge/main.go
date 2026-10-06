//go:build js && wasm

// Command offline-judge is a Python / C++ judge that runs entirely in the browser.
package main

import (
	"fmt"
	"log"
	"sync"
	"syscall/js"
	"time"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"
	"github.com/voilelab/toolgui/toolgui/tgwasm"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/problems"
)

// Inputs/outputs longer than this are cut in the result view.
const showLimit = 2000

type runner interface {
	judge.Runner
	Ready() bool
}

type lang struct {
	name    string
	id      string
	code    string // default code
	loading string // shown while the runtime loads
	newRun  func() runner
	once    sync.Once
	run     runner
}

// runner starts the runtime on first use, so unused ones aren't downloaded.
func (l *lang) runner() runner {
	l.once.Do(func() { l.run = l.newRun() })
	return l.run
}

var (
	probs []*problems.Problem
	langs = []*lang{
		{
			name:    "Python",
			id:      "py",
			code:    "import sys\ninput = sys.stdin.readline\n\n",
			loading: "載入中（首次約需數秒）",
			newRun:  func() runner { return NewPyRunner(assetURL("pyworker.mjs")) },
		},
		{
			name: "C++",
			id:   "cpp",
			code: "#include <bits/stdc++.h>\nusing namespace std;\n\nint main() {\n" +
				"    ios::sync_with_stdio(false);\n    cin.tie(nullptr);\n\n}\n",
			loading: "載入中（首次需下載約 27 MB 的 clang）",
			newRun: func() runner {
				return NewCppRunner(assetURL("cppcompile.mjs"), assetURL("cpprun.mjs"))
			},
		},
	}
)

const intro = `完全在瀏覽器裡執行的 Python / C++ 解題系統，不需要後端，載入後可離線使用。

- 從下方或左側選一題，寫好程式後「提交」即可評測
- 「自訂輸入」可以用自己的輸入先跑跑看
- 測資不保密，失敗時會顯示第一筆錯誤的完整輸入與輸出`

// Index is the home page: intro and problem list.
func Index(p *tgframe.Params) error {
	tgcomp.Title(p.Main, "Offline Judge")
	tgcomp.Markdown(p.Main, intro)

	tgcomp.Subtitle(p.Main, "題目")
	for _, pr := range probs {
		tgcomp.Link(p.Main, pr.Title, "#/"+pr.ID)
		tgcomp.Caption(p.Main, fmt.Sprintf("時間限制 %d ms", pr.TimeLimit.Milliseconds()))
	}
	return nil
}

// problemPage shows one problem with its editor and judge.
func problemPage(pr *problems.Problem) tgframe.RunFunc {
	return func(p *tgframe.Params) error {
		return showProblem(p, pr)
	}
}

func showProblem(p *tgframe.Params, pr *problems.Problem) error {
	names := make([]string, len(langs))
	for i, l := range langs {
		names[i] = l.name
	}
	li := tgcomp.Select(p.Sidebar, "語言", names,
		(&tgcomp.SelectConf{}).SetDefault(memo.getLang()))
	if li == nil {
		return nil
	}
	memo.setLang(*li)
	lg := langs[*li]
	run := lg.runner()

	if run.Ready() {
		tgcomp.Caption(p.Sidebar, lg.name+" 環境：就緒")
	} else {
		tgcomp.Caption(p.Sidebar, lg.name+" 環境："+lg.loading)
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

	// One textarea per problem and language, so switching keeps the code.
	key := lg.id + "_" + pr.ID
	code := tgcomp.Textarea(p.Main, "程式碼（"+lg.name+"）", &tgcomp.TextareaConf{
		ID:      "code_" + key,
		Height:  16,
		Default: memo.getText("code_"+key, lg.code),
	})
	memo.setText("code_"+key, code)

	submitTab, customTab := tgcomp.Tab2(p.Main, "提交", "自訂輸入")
	submitPanel(p, submitTab, run, key, pr, code)
	customPanel(p, customTab, run, key, pr, code)
	return nil
}

func submitPanel(p *tgframe.Params, c *tgframe.Container, run runner, key string,
	pr *problems.Problem, code string) {

	if tgcomp.Button(c, "提交", &tgcomp.ButtonConf{ID: "submit_" + key}) {
		st := tgcomp.Status(c, "評測中…")
		rep, err := judge.Judge(p.Context, run, code, pr.Cases, pr.TimeLimit,
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
		memo.setReport(key, &rep)
	}

	rep := memo.getReport(key)
	if rep == nil {
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
	if rep.Verdict == judge.CE {
		tgcomp.Code(c, cut(rep.CompileError), &tgcomp.CodeConf{Language: "text"})
		return
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

func customPanel(p *tgframe.Params, c *tgframe.Container, run runner, key string,
	pr *problems.Problem, code string) {

	stdin := tgcomp.Textarea(c, "輸入", &tgcomp.TextareaConf{
		ID:      "stdin_" + key,
		Height:  6,
		Default: memo.getText("stdin_"+key, pr.Cases[0].Input),
	})
	memo.setText("stdin_"+key, stdin)
	if !tgcomp.Button(c, "執行", &tgcomp.ButtonConf{ID: "run_" + key}) {
		return
	}

	done := tgcomp.Spinner(c, "執行中…")
	res, err := run.Run(p.Context, code, stdin, pr.TimeLimit)
	done()
	if err != nil {
		tgcomp.MessageDanger(c, err.Error())
		return
	}

	switch {
	case res.Status == judge.RunCompileError:
		tgcomp.MessageDanger(c, "CE：編譯錯誤")
		tgcomp.Code(c, cut(res.Stderr), &tgcomp.CodeConf{Language: "text"})
		return
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

// memo keeps inputs and results across pages,
// since toolgui starts each page with an empty state.
var memo = &store{text: map[string]string{}, reports: map[string]*judge.Report{}}

type store struct {
	mu      sync.Mutex
	lang    int
	text    map[string]string
	reports map[string]*judge.Report
}

func (s *store) getLang() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lang
}

func (s *store) setLang(i int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lang = i
}

func (s *store) getText(key, def string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.text[key]; ok {
		return v
	}
	return def
}

func (s *store) setText(key, v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.text[key] = v
}

func (s *store) getReport(key string) *judge.Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reports[key]
}

func (s *store) setReport(key string, r *judge.Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[key] = r
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

// assetURL resolves a web/ file against this (Go) worker's script,
// which toolgui-wasm puts in static/.
func assetURL(name string) string {
	href := js.Global().Get("location").Get("href")
	return js.Global().Get("URL").New("../assets/"+name, href).Get("href").String()
}

func main() {
	var err error
	probs, err = problems.All()
	if err != nil {
		log.Fatal(err)
	}
	// Python is the default language: start loading it now.
	langs[0].runner()

	app := tgframe.NewApp()
	app.SetTitle("Offline Judge")
	app.AddPageByConfig(&tgframe.PageConfig{Name: "index", Title: "首頁", Emoji: "🏠"}, Index)
	for _, pr := range probs {
		app.AddPage(pr.ID, pr.Title, problemPage(pr))
	}
	app.SetHashPageNameMode(true)
	tgwasm.NewExecutor(app).Run()
}
