//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/internal/source"
	"github.com/mudream4869/offline-judge/problems"
)

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
	starBox(p, pr.ID)

	// No hints or solution tags in a contest.
	inContest := contestOf(pr.ID)
	if inContest != nil {
		tgcomp.MessageInfo(p.Main, fmt.Sprintf("模擬賽「%s」進行中，%s 結束",
			inContest.Title, inContest.End.Format("15:04")))
		tgcomp.PageLink(p.Main, "查看模擬賽計分板", "contest", nil)
	}
	tgcomp.Markdown(p.Main, pr.Statement)
	info := fmt.Sprintf("時間限制（%s）：%d ms", lg.name, pr.TimeLimitFor(lg.id).Milliseconds())
	if pr.Version != "" {
		info += "，版本：" + pr.Version
	}
	if len(pr.Tags) > 0 {
		info += "，標籤：" + strings.Join(pr.Tags, "、")
	}
	showSol := showSolutionTags() && inContest == nil
	if showSol && len(pr.SolutionTags) > 0 {
		info += "，解法標籤：" + strings.Join(pr.SolutionTags, "、")
	}
	tgcomp.Caption(p.Main, info)
	if pr.Checker != "" {
		tgcomp.Caption(p.Main, "答案不唯一，由題目的 checker 判定；範例輸出只是其中一種")
	}
	if pr.Interactor != "" {
		tgcomp.Caption(p.Main, "互動題：程式與題目的互動程式一問一答")
	}
	if len(pr.Graders) > 0 {
		tgcomp.Caption(p.Main, "函式題：只要寫題目要求的函式，輸入輸出由題目的 grader 處理；"+
			"支援 "+graderLangs(pr))
	}
	if c := compareCaption(pr.Compare); c != "" {
		tgcomp.Caption(p.Main, c)
	}

	inLabel, outLabel := "輸入", "輸出"
	if pr.Interactor != "" {
		inLabel, outLabel = "互動程式的輸入", "互動過程"
	}
	for i, c := range pr.Samples() {
		tgcomp.Subtitle(p.Main, fmt.Sprintf("範例 %d", i+1))
		in, out := tgcomp.EqColumn2(p.Main, &tgcomp.ColumnConf{ID: "sample_" + c.Name})
		tgcomp.Text(in, inLabel)
		tgcomp.Code(in, c.Input, &tgcomp.CodeConf{Language: "text"})
		tgcomp.Text(out, outLabel)
		tgcomp.Code(out, c.Output, &tgcomp.CodeConf{Language: "text"})
	}
	if len(pr.Subtasks) > 0 {
		tgcomp.Subtitle(p.Main, "子任務")
		tgcomp.Markdown(p.Main, subtaskTable(pr.Subtasks))
	}

	if pr.Hint != "" && inContest == nil {
		h := tgcomp.Expand(p.Main, "提示", false, &tgcomp.ExpandConf{ID: "hint_" + pr.ID})
		tgcomp.Markdown(h, pr.Hint)
	}
	if !showSol && len(pr.SolutionTags) > 0 && inContest == nil {
		h := tgcomp.Expand(p.Main, "解法標籤（點開會暴雷）", false, &tgcomp.ExpandConf{ID: "soltags_" + pr.ID})
		tgcomp.Text(h, strings.Join(pr.SolutionTags, "、"))
	}
	noteSection(p.Main, pr.ID)
	if u := reportURL(pr); u != "" {
		tgcomp.Link(p.Main, "回報題目問題（GitHub issue）", u)
	}

	tgcomp.Divider(p.Main)

	// One editor per problem and language, so switching keeps the code.
	key := lg.id + "_" + pr.ID
	resetConf := &tgcomp.ButtonConf{ID: "reset_code_" + key}
	reset := tgcomp.ButtonClicked(p.Main, resetLabel, resetConf)
	if reset {
		memo.setText("code_"+key, startCode(lg, pr))
	}
	code := tgcomp.CodeInput(p.Main, "程式碼（"+lg.name+"）", &tgcomp.CodeInputConf{
		ID:        "code_" + key,
		Language:  lg.hl,
		Height:    16,
		MaxHeight: 40,
		Default:   memo.getText("code_"+key, startCode(lg, pr)),
		ResetKey:  codeResetKey(key, reset),
	})
	memo.setText("code_"+key, code)
	tgcomp.Button(p.Main, resetLabel, resetConf)

	// Before drawing, so every panel sees the deletion.
	for _, s := range memo.submissions(pr.ID) {
		if tgcomp.ButtonClicked(p.Main, delLabel, delConf(s.ID)) {
			if err := memo.deleteSubmission(s.ID); err != nil {
				tgcomp.MessageDanger(p.Main, "刪除失敗："+err.Error())
			}
		}
	}

	submitTab, customTab, stressTab, histTab := tgcomp.Tab4(p.Main, "提交", "自訂輸入", "對拍", "紀錄")
	submitPanel(p, submitTab, lg, key, pr, code)
	customPanel(p, customTab, run, lg, key, pr, code)
	stressPanel(p, stressTab, lg, key, pr, code)
	historyPanel(p.Context, histTab, pr)
	return nil
}

func submitPanel(p *tgframe.Params, c *tgframe.Container, lg *lang,
	key string, pr *problems.Problem, code string) {

	if !canRun(c, lg, pr) {
		return
	}
	if tgcomp.Button(c, "提交", &tgcomp.ButtonConf{ID: "submit_" + key}) {
		if judgeAndSave(p.Context, c, lg, pr, code) == nil {
			return
		}
	}
	if samples := pr.Samples(); len(samples) > 0 &&
		tgcomp.Button(c, "測試範例", &tgcomp.ButtonConf{ID: "samples_" + key}) {
		if rep := judgeCases(p.Context, c, lg, pr, code, samples); rep != nil {
			tgcomp.Caption(c, fmt.Sprintf("只跑了 %d 筆範例，結果不存成提交紀錄", len(samples)))
			showReport(c, pr, rep, "samples_"+key)
		}
		return
	}

	// Latest submission in this language.
	for _, s := range memo.submissions(pr.ID) {
		if s.Lang == lg.id {
			warnUnsavedSubmission(c, s)
			showReport(c, pr, &s.Report, key)
			return
		}
	}
}

// judgeAndSave judges code in lg against pr, showing progress in c, and
// stores the submission. On failure it says why in c and returns nil.
func judgeAndSave(ctx context.Context, c *tgframe.Container, lg *lang,
	pr *problems.Problem, code string) *submission {

	rep := judgeCases(ctx, c, lg, pr, code, pr.Cases)
	if rep == nil {
		return nil
	}
	sub := &submission{
		Problem: pr.ID,
		Version: pr.Version,
		Lang:    lg.id,
		Code:    code,
		At:      time.Now(),
		Report:  *rep,
	}
	memo.addSubmission(sub)
	return sub
}

// judgeCases judges code in lg against cases of pr, showing progress in c.
// Subtasks count only when cases are all of pr's. On failure it says why in
// c and returns nil. The report is slim.
func judgeCases(ctx context.Context, c *tgframe.Container, lg *lang,
	pr *problems.Problem, code string, cases []judge.Case) *judge.Report {

	spec := judge.Spec{Limit: pr.TimeLimitFor(lg.id), Interactor: pr.Interactor,
		Grader: pr.Graders[lg.id], Compare: pr.Compare}
	if len(cases) == len(pr.Cases) {
		spec.Subtasks = pr.JudgeSubtasks()
	}
	if pr.Checker != "" {
		spec.Check = checkRunner().Checker(pr.Checker)
	}
	st := tgcomp.Status(c, "評測中…")
	rep, err := judge.Judge(ctx, lg.runner(), code, cases, spec,
		func(done int, cr judge.CaseResult) {
			st.Update(fmt.Sprintf("評測中 %d/%d", done, len(cases)))
			st.Write(fmt.Sprintf("%s：%s（%s）", cr.Name, cr.Verdict, fmtTime(cr.Time)))
		})
	if err != nil {
		st.Error("評測失敗")
		tgcomp.MessageDanger(c, err.Error())
		return nil
	}
	st.Complete("評測完成")
	rep = slim(rep)
	return &rep
}

// rejudgeButton offers to judge s again against pr, the current version,
// when s was judged against another. The new submission is shown in c.
func rejudgeButton(ctx context.Context, c *tgframe.Container, pr *problems.Problem,
	s *submission, id string) {

	lg := langByID(s.Lang)
	if pr == nil || !outdated(s, pr.Version) || lg.newRun == nil {
		return
	}
	if !tgcomp.Button(c, "用目前版本重新評測", &tgcomp.ButtonConf{ID: "rejudge_" + id}) {
		return
	}
	if !canRun(c, lg, pr) {
		return
	}
	if ns := judgeAndSave(ctx, c, lg, pr, s.Code); ns != nil {
		tgcomp.Caption(c, "已存成 "+submissionNumber(ns.ID))
		warnUnsavedSubmission(c, ns)
		showReport(c, pr, &ns.Report, "rejudged_"+id)
	}
}

// canRun reports whether lg can run pr, saying why not in c.
func canRun(c *tgframe.Container, lg *lang, pr *problems.Problem) bool {
	if pr.Unsupported != "" {
		tgcomp.MessageWarning(c, "無法評測："+pr.Unsupported)
		return false
	}
	if pr.Interactor != "" && !lg.interactive {
		tgcomp.MessageWarning(c, lg.name+" 還不支援互動題，請改用其他語言")
		return false
	}
	if len(pr.Graders) > 0 && pr.Graders[lg.id] == "" {
		tgcomp.MessageWarning(c, "這題沒有 "+lg.name+" 的 grader，請改用 "+graderLangs(pr))
		return false
	}
	return true
}

const resetLabel = "還原預設程式碼"

var (
	resetMu    sync.Mutex
	codeResets = map[string]int{} // by editor key
)

// codeResetKey is the editor's ResetKey; bump changes it, which resets the editor.
func codeResetKey(key string, bump bool) string {
	resetMu.Lock()
	defer resetMu.Unlock()
	if bump {
		codeResets[key]++
	}
	return strconv.Itoa(codeResets[key])
}

// startCode is the editor's code before any edit: the problem's template, if any.
func startCode(lg *lang, pr *problems.Problem) string {
	if t := pr.Templates[lg.id]; t != "" {
		return t
	}
	return userTemplate(lg)
}

// graderLangs names the languages pr has graders for.
func graderLangs(pr *problems.Problem) string {
	var names []string
	for _, l := range langs {
		if pr.Graders[l.id] != "" {
			names = append(names, l.name)
		}
	}
	return strings.Join(names, "、")
}

// historyPanel lists recent submissions of a problem in all languages.
func historyPanel(ctx context.Context, c *tgframe.Container, pr *problems.Problem) {
	subs := memo.submissions(pr.ID)
	if len(subs) == 0 {
		tgcomp.Caption(c, "還沒有提交紀錄")
		return
	}
	tgcomp.Caption(c, fmt.Sprintf("最近 %d 筆提交，存在這個瀏覽器裡", len(subs)))
	for _, s := range subs {
		lg := langByID(s.Lang)
		title := fmt.Sprintf("%s  %s  %s  %s", submissionNumber(s.ID), s.At.Format("2006-01-02 15:04:05"),
			lg.name, resultText(&s.Report))
		if s.Report.Verdict != judge.CE {
			title += "  " + fmtTime(maxTime(&s.Report))
		}
		if outdated(s, pr.Version) {
			title += "  （舊版題目）"
		}
		id := fmt.Sprintf("sub_%d", s.ID)
		box := tgcomp.Expand(c, title, false, &tgcomp.ExpandConf{ID: id})
		warnUnsavedSubmission(box, s)
		tgcomp.Code(box, s.Code, &tgcomp.CodeConf{Language: lg.hl})
		showReport(box, pr, &s.Report, id)
		rejudgeButton(ctx, box, pr, s, id)
		tgcomp.Button(box, delLabel, delConf(s.ID))
	}
}

func customPanel(p *tgframe.Params, c *tgframe.Container, run runner, lg *lang,
	key string, pr *problems.Problem, code string) {

	if !canRun(c, lg, pr) {
		return
	}
	label := "輸入"
	if pr.Interactor != "" {
		label = "互動程式的輸入"
	}
	stdin := tgcomp.Textarea(c, label, &tgcomp.TextareaConf{
		ID:      "stdin_" + key,
		Height:  6,
		Default: memo.getText("stdin_"+key, pr.Cases[0].Input),
	})
	memo.setText("stdin_"+key, stdin)
	if !tgcomp.Button(c, "執行", &tgcomp.ButtonConf{ID: "run_" + key}) {
		return
	}

	limit := pr.TimeLimitFor(lg.id)
	done := tgcomp.Spinner(c, "執行中…")
	res, err := run.Run(p.Context, code, judge.Input{Stdin: stdin, Interactor: pr.Interactor,
		Grader: pr.Graders[lg.id]}, limit)
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
	case res.Status == judge.RunTimeout || res.Time > limit:
		tgcomp.MessageWarning(c, "TLE："+fmtTime(res.Time))
	case res.Status == judge.RunOutputLimit:
		tgcomp.MessageDanger(c, "OLE："+verdictName(judge.OLE))
	case res.Judged != nil && !res.Judged.OK:
		tgcomp.MessageDanger(c, "WA："+res.Judged.Message)
	case res.Status == judge.RunError:
		tgcomp.MessageDanger(c, "RE："+fmtTime(res.Time))
	case res.Judged != nil:
		tgcomp.MessageSuccess(c, "AC：互動程式判定正確（"+fmtTime(res.Time)+"）")
	default:
		tgcomp.MessageInfo(c, "執行完成："+fmtTime(res.Time))
	}
	tgcomp.Text(c, stdoutLabel(pr))
	tgcomp.Code(c, cut(res.Stdout), &tgcomp.CodeConf{Language: "text"})
	if res.Stderr != "" {
		tgcomp.Text(c, "stderr")
		tgcomp.Code(c, cut(res.Stderr), &tgcomp.CodeConf{Language: "text"})
	}
}

// compareCaption explains a comparison other than the default, or returns "".
func compareCaption(cm judge.Compare) string {
	switch cm.Mode {
	case judge.CompareStrict:
		return "輸出要逐字元相同，包含空白與換行"
	case judge.CompareWhite:
		return "以空白分隔逐項比對，空白的數量不影響判定，但換行要相同"
	case judge.CompareFloat:
		err := "絕對或相對誤差"
		switch {
		case !cm.Rel:
			err = "絕對誤差"
		case !cm.Abs:
			err = "相對誤差"
		}
		return fmt.Sprintf("輸出的小數與答案的%s在 %g 以內即可，其他項要相同", err, cm.Eps)
	case judge.CompareTokens:
		var parts []string
		if cm.SpaceSensitive {
			parts = append(parts, "以空白分隔逐項比對，空白要完全相同")
		} else {
			parts = append(parts, "以空白分隔逐項比對，空白與換行不影響判定")
		}
		if !cm.CaseSensitive {
			parts = append(parts, "不分大小寫")
		}
		switch {
		case cm.Abs && cm.Rel && cm.AbsTol == cm.RelTol:
			parts = append(parts, fmt.Sprintf("小數的絕對或相對誤差在 %g 以內即可", cm.AbsTol))
		case cm.Abs && cm.Rel:
			parts = append(parts, fmt.Sprintf("小數的絕對誤差在 %g 或相對誤差在 %g 以內即可", cm.AbsTol, cm.RelTol))
		case cm.Abs:
			parts = append(parts, fmt.Sprintf("小數的絕對誤差在 %g 以內即可", cm.AbsTol))
		case cm.Rel:
			parts = append(parts, fmt.Sprintf("小數的相對誤差在 %g 以內即可", cm.RelTol))
		}
		return strings.Join(parts, "，")
	}
	return ""
}

// stdoutLabel names a run's Stdout, which is a transcript for an interactive problem.
func stdoutLabel(pr *problems.Problem) string {
	if pr.Interactor != "" {
		return "互動過程（→ 你的輸出，← 互動程式）"
	}
	return "你的輸出"
}

// subtaskTable is a markdown table of subs, so constraints can hold LaTeX.
func subtaskTable(subs []problems.Subtask) string {
	cell := strings.NewReplacer("|", "\\|", "\n", " ")
	var b strings.Builder
	b.WriteString("| 子任務 | 分數 | 測資 | 限制 |\n| --- | --- | --- | --- |\n")
	for i, st := range subs {
		fmt.Fprintf(&b, "| %d | %g | %s | %s |\n", i+1, st.Score,
			strings.Join(st.Cases, "、"), cell.Replace(st.Constraints))
	}
	return b.String()
}

// reportURL links to a new issue about pr on the source's repository,
// prefilled with what identifies the problem; "" if the source isn't valid.
func reportURL(pr *problems.Problem) string {
	ref, ok := refOf(pr.ID)
	if !ok {
		return ""
	}
	r, err := source.Parse(ref.set.URL)
	if err != nil {
		return ""
	}
	at := ""
	if c := ref.set.Commit(); c != "" {
		at = "（commit " + c[:7] + "）"
	}
	body := fmt.Sprintf("題目：%s（%s）\n版本：%s\n來源：%s%s\n\n## 問題描述\n\n",
		pr.Title, ref.id, pr.Version, ref.set.URL, at)
	return source.IssueURL(r, "題目 "+problemNumber(ref.id)+" "+pr.Title+"：", body)
}
