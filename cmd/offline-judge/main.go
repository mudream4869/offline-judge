//go:build js && wasm

// Command offline-judge is a Python / C++ / JavaScript judge that runs entirely in the browser.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"syscall/js"
	"time"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgcomp/tcutil"
	"github.com/voilelab/toolgui/toolgui/tgframe"
	"github.com/voilelab/toolgui/toolgui/tgwasm"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/source"
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
	hl      string // code highlight language
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
	langs = []*lang{
		{
			name:    "Python",
			id:      "py",
			hl:      "python",
			code:    "import sys\ninput = sys.stdin.readline\n\n",
			loading: "載入中（首次約需數秒）",
			newRun:  func() runner { return NewCodeRunner(assetURL("pyworker.mjs"), "Python") },
		},
		{
			name: "C++",
			id:   "cpp",
			hl:   "cpp",
			code: "#include <bits/stdc++.h>\nusing namespace std;\n\nint main() {\n" +
				"    ios::sync_with_stdio(false);\n    cin.tie(nullptr);\n\n    return 0;\n}\n",
			loading: "載入中（首次需下載約 27 MB 的 clang）",
			newRun: func() runner {
				return NewCppRunner(assetURL("cppcompile.mjs"), assetURL("cpprun.mjs"))
			},
		},
		{
			name:    "JavaScript",
			id:      "js",
			hl:      "javascript",
			code:    "const lines = require('fs').readFileSync(0, 'utf8').split('\\n')\n\n",
			loading: "載入中",
			newRun:  func() runner { return NewCodeRunner(assetURL("jsrun.mjs"), "JavaScript") },
		},
	}
)

const intro = `完全在瀏覽器裡執行的 Python / C++ / JavaScript 解題系統，不需要後端。

- 到「題目列表」點一題，寫好程式後「提交」即可評測
- 「自訂輸入」可以用自己的輸入先跑跑看
- 測資不保密，失敗時會顯示第一筆錯誤的完整輸入與輸出
- 題目從 GitHub 下載，打開過的題目可離線使用；來源可在「設定」更改`

// Index is the home page.
func Index(p *tgframe.Params) error {
	tgcomp.Title(p.Main, "Offline Judge")
	tgcomp.Markdown(p.Main, intro)
	tgcomp.Link(p.Main, "前往題目列表", "#/problems")
	return nil
}

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

	ids := make([]string, len(es))
	rows := make([][]string, len(es))
	for i, e := range es {
		off := ""
		if e.Cached {
			off = "✓"
		}
		ids[i] = e.ID
		rows[i] = []string{e.Title, fmtTime(e.TimeLimit), off}
	}
	slot := tgcomp.Empty(p.Main)
	var sel []int
	slot.With(func(c *tgframe.Container) {
		tgcomp.Title(c, "題目列表")
		tgcomp.Caption(c, "點一題開始作答")
		sel = tgcomp.DataFrame(c, []string{"題目", "時間限制", "可離線"}, rows,
			(&tgcomp.DataFrameConf{
				Base:      tgframe.Base{ID: "problem_list"},
				PageSize:  100,
				Selection: tgcomp.SelectionModeSingle,
				RowKeys:   ids,
			}).SetSortable(false))
	})
	if len(sel) == 0 {
		return nil
	}
	// Clearing drops the list's pick too, so it comes back unpicked.
	slot.Clear()
	p.State.Set(pickedKey, ids[sel[0]])
	return &es[sel[0]]
}

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

	// One editor per problem and language, so switching keeps the code.
	key := lg.id + "_" + pr.ID
	code := tgcomp.CodeInput(p.Main, "程式碼（"+lg.name+"）", &tgcomp.CodeInputConf{
		ID:        "code_" + key,
		Language:  lg.hl,
		Height:    16,
		MaxHeight: 40,
		Default:   memo.getText("code_"+key, lg.code),
	})
	memo.setText("code_"+key, code)

	// Before drawing, so every panel sees the deletion.
	for _, s := range memo.submissions(pr.ID) {
		if tgcomp.ButtonClicked(p.Main, delLabel, delConf(s.ID)) {
			memo.deleteSubmission(pr.ID, s.ID)
		}
	}

	submitTab, customTab, histTab := tgcomp.Tab3(p.Main, "提交", "自訂輸入", "紀錄")
	submitPanel(p, submitTab, run, lg, key, pr, code)
	customPanel(p, customTab, run, key, pr, code)
	historyPanel(histTab, pr)
	return nil
}

func submitPanel(p *tgframe.Params, c *tgframe.Container, run runner, lg *lang,
	key string, pr *problems.Problem, code string) {

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
		memo.addSubmission(&submission{
			Problem: pr.ID,
			Lang:    lg.id,
			Code:    code,
			At:      time.Now(),
			Report:  slim(rep),
		})
	}

	// Latest submission in this language.
	for _, s := range memo.submissions(pr.ID) {
		if s.Lang == lg.id {
			showReport(c, pr, &s.Report, key)
			return
		}
	}
}

// historyPanel lists recent submissions of a problem in all languages.
func historyPanel(c *tgframe.Container, pr *problems.Problem) {
	subs := memo.submissions(pr.ID)
	if len(subs) == 0 {
		tgcomp.Caption(c, "還沒有提交紀錄")
		return
	}
	tgcomp.Caption(c, fmt.Sprintf("最近 %d 筆提交，存在這個瀏覽器裡", len(subs)))
	for _, s := range subs {
		lg := langByID(s.Lang)
		title := fmt.Sprintf("#%d  %s  %s  %s", s.ID, s.At.Format("2006-01-02 15:04:05"),
			lg.name, s.Report.Verdict)
		if s.Report.Verdict != judge.CE {
			title += "  " + fmtTime(maxTime(&s.Report))
		}
		id := fmt.Sprintf("sub_%d", s.ID)
		box := tgcomp.Expand(c, title, false, &tgcomp.ExpandConf{ID: id})
		tgcomp.Code(box, s.Code, &tgcomp.CodeConf{Language: lg.hl})
		showReport(box, pr, &s.Report, id)
		tgcomp.Button(box, delLabel, delConf(s.ID))
	}
}

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
			cr.Stdout, cr.Stderr = cut(cr.Stdout), cut(cr.Stderr)
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
		// The tests may have changed since, or the problem isn't loaded.
		if pr == nil || i >= len(pr.Cases) || pr.Cases[i].Name != cr.Name {
			break
		}
		tc := pr.Cases[i]
		box := tgcomp.Expand(c, "第一筆失敗："+cr.Name, true,
			&tgcomp.ExpandConf{ID: "fail_" + id})
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

// memo caches drafts and submissions, backed by IndexedDB.
// toolgui starts each page with an empty state, so pages read from here.
var memo = &store{text: map[string]string{}, subs: map[string][]*submission{}}

// Submissions kept per problem in memory and shown in history.
const historyLimit = 50

type store struct {
	mu   sync.Mutex
	text map[string]string
	subs map[string][]*submission // by problem, newest first
	all  []*submission            // every problem, newest first; nil until loaded
}

func (s *store) getLang() int {
	id := s.getText("lang", langs[0].id)
	for i, l := range langs {
		if l.id == id {
			return i
		}
	}
	return 0
}

func (s *store) setLang(i int) {
	s.setText("lang", langs[i].id)
}

// getText returns the saved text, or def if there is none.
func (s *store) getText(key, def string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.text[key]; ok {
		return v
	}
	v, ok := loadDraft(key)
	if !ok {
		// Cached but not saved: an untouched draft follows a new default.
		v = def
	}
	s.text[key] = v
	return v
}

func (s *store) setText(key, v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.text[key]; ok && old == v {
		return
	}
	s.text[key] = v
	saveDraft(key, v)
}

func (s *store) submissions(problem string) []*submission {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadSubs(problem)
}

func (s *store) loadSubs(problem string) []*submission {
	if subs, ok := s.subs[problem]; ok {
		return subs
	}
	subs, err := loadSubmissions(problem, historyLimit)
	logDBErr("讀取", err)
	s.subs[problem] = subs
	return subs
}

// allSubmissions returns the submissions of every problem, newest first.
func (s *store) allSubmissions() []*submission {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAll()
}

func (s *store) loadAll() []*submission {
	if s.all == nil {
		all, err := loadAllSubmissions()
		logDBErr("讀取", err)
		s.all = append([]*submission{}, all...) // non-nil: loaded
	}
	return s.all
}

func (s *store) addSubmission(sub *submission) {
	s.mu.Lock()
	defer s.mu.Unlock()
	subs := s.loadSubs(sub.Problem)
	if err := saveSubmission(sub); err != nil {
		logDBErr("寫入", err)
		// Unsaved: number it after the newest so it still shows.
		sub.ID = 1
		if all := s.loadAll(); len(all) > 0 {
			sub.ID = all[0].ID + 1
		}
	}
	subs = append([]*submission{sub}, subs...)
	if len(subs) > historyLimit {
		subs = subs[:historyLimit]
	}
	s.subs[sub.Problem] = subs
	if s.all != nil {
		s.all = append([]*submission{sub}, s.all...)
	}
}

func (s *store) deleteSubmission(problem string, id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleteSubmission(id)
	subs := s.loadSubs(problem)
	for i, sub := range subs {
		if sub.ID == id {
			s.subs[problem] = append(subs[:i:i], subs[i+1:]...)
			break
		}
	}
	for i, sub := range s.all {
		if sub.ID == id {
			s.all = append(s.all[:i:i], s.all[i+1:]...)
			break
		}
	}
}

func langByID(id string) *lang {
	for _, l := range langs {
		if l.id == id {
			return l
		}
	}
	return &lang{name: id, id: id}
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
	// Python is the default language: start loading it now.
	langs[0].runner()

	app := tgframe.NewApp()
	app.SetTitle("Offline Judge")
	app.AddPageByConfig(&tgframe.PageConfig{Name: "index", Title: "首頁", Emoji: "🏠"}, Index)
	app.AddPageByConfig(&tgframe.PageConfig{Name: "problems", Title: "題目列表", Emoji: "📚"}, Problems)
	app.AddPageByConfig(&tgframe.PageConfig{Name: "submissions", Title: "提交紀錄", Emoji: "📝"}, Submissions)
	app.AddPageByConfig(&tgframe.PageConfig{Name: "settings", Title: "設定", Emoji: "⚙️"}, Settings)
	app.SetHashPageNameMode(true)
	tgwasm.NewExecutor(app).Run()
}
