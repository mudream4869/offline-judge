//go:build js && wasm

package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/judge"
	"github.com/mudream4869/offline-judge/problems"
)

// stressSlowLimit is the time limit of the generator and the brute force.
const stressSlowLimit = 5 * time.Second

// stressPanel stress-tests code against a brute force on generated inputs.
func stressPanel(p *tgframe.Params, c *tgframe.Container, lg *lang,
	key string, pr *problems.Problem, code string) {

	if !canRun(c, lg, pr) {
		return
	}
	if pr.Interactor != "" {
		tgcomp.MessageInfo(c, "互動題沒有固定的輸入，不能對拍")
		return
	}
	tgcomp.Caption(c, "產生器讀入一個種子、輸出一筆輸入；暴力解與你的程式各跑一次，"+
		"輸出不同（依題目的比對方式或 checker）或執行失敗就停下。用同一種語言，結果不存成提交紀錄")

	brute := tgcomp.CodeInput(c, "暴力解（"+lg.name+"）", &tgcomp.CodeInputConf{
		ID:        "brute_" + key,
		Language:  lg.hl,
		Height:    8,
		MaxHeight: 30,
		Default:   memo.getText("brute_"+key, startCode(lg, pr)),
	})
	memo.setText("brute_"+key, brute)
	if len(pr.Graders) > 0 {
		tgcomp.Caption(c, "函式題：暴力解也只寫函式，跟你的程式一樣由 grader 呼叫")
	}
	gen := tgcomp.CodeInput(c, "測資產生器（"+lg.name+"）", &tgcomp.CodeInputConf{
		ID:        "gen_" + key,
		Language:  lg.hl,
		Height:    8,
		MaxHeight: 30,
		Default:   memo.getText("gen_"+key, lg.gen),
	})
	memo.setText("gen_"+key, gen)

	one := 1
	cols := tgcomp.Column(c, 2, &tgcomp.ColumnConf{ID: "stress_opts_" + key})
	first, _ := tgcomp.Number(cols[0], "起始種子", &tgcomp.NumberConf[int]{
		Base:    tgframe.Base{ID: "stress_seed_" + key},
		Default: 1,
	})
	maxRounds := 10000
	rounds, _ := tgcomp.Number(cols[1], "回合數", &tgcomp.NumberConf[int]{
		Base:    tgframe.Base{ID: "stress_rounds_" + key},
		Default: 100,
		Min:     &one,
		Max:     &maxRounds,
	})
	if !tgcomp.Button(c, "開始對拍", &tgcomp.ButtonConf{ID: "stress_" + key}) {
		return
	}

	spec := judge.StressSpec{Gen: gen, Brute: brute, Code: code, Grader: pr.Graders[lg.id],
		Limit: pr.TimeLimitFor(lg.id), SlowLimit: max(stressSlowLimit, pr.TimeLimitFor(lg.id)),
		Compare: pr.Compare}
	if pr.Checker != "" {
		spec.Check = checkRunner().Checker(pr.Checker)
	}
	st := tgcomp.Status(c, "對拍中…")
	f, err := judge.Stress(p.Context, lg.runner(), spec, first, max(rounds, 1), func(done int) {
		st.Update(fmt.Sprintf("對拍中 %d/%d", done, rounds))
	})
	if err != nil {
		st.Error("對拍失敗")
		tgcomp.MessageDanger(c, err.Error())
		return
	}
	if f == nil {
		st.Complete("對拍完成")
		tgcomp.MessageSuccess(c, fmt.Sprintf("種子 %d–%d 共 %d 回合，輸出都相同", first, first+rounds-1, rounds))
		return
	}
	st.Error("找到不同")
	showStressFailure(c, pr, f, key)
}

var stressWho = map[string]string{"gen": "測資產生器", "brute": "暴力解", "code": "你的程式"}

// showStressFailure shows the round that failed.
func showStressFailure(c *tgframe.Container, pr *problems.Problem, f *judge.StressFailure, key string) {
	seed := "種子 " + strconv.Itoa(f.Seed)
	if f.Who != "" {
		tgcomp.MessageDanger(c, fmt.Sprintf("%s：%s %s（%s）", seed, stressWho[f.Who], f.Verdict, verdictName(f.Verdict)))
	} else {
		tgcomp.MessageDanger(c, seed+"：你的程式與暴力解的輸出不同")
	}
	if f.Who == "gen" {
		tgcomp.Text(c, "stderr")
		tgcomp.Code(c, cut(f.Stderr), &tgcomp.CodeConf{Language: "text"})
		return
	}
	tgcomp.Text(c, "輸入")
	tgcomp.Code(c, cut(f.Input), &tgcomp.CodeConf{Language: "text"})
	switch f.Who {
	case "":
		if pr.Checker == "" {
			sideBySide(c, pr.Compare, f.Got, f.Want, "暴力解的輸出", "stress_"+key)
		} else {
			tgcomp.Text(c, "暴力解的輸出")
			tgcomp.Code(c, cut(f.Want), &tgcomp.CodeConf{Language: "text"})
			tgcomp.Text(c, "你的輸出")
			tgcomp.Code(c, cut(f.Got), &tgcomp.CodeConf{Language: "text"})
		}
		tgcomp.Text(c, "比對結果")
		tgcomp.Code(c, f.Message, &tgcomp.CodeConf{Language: "text"})
	default:
		if f.Who == "code" {
			tgcomp.Text(c, "暴力解的輸出")
			tgcomp.Code(c, cut(f.Want), &tgcomp.CodeConf{Language: "text"})
		}
		if f.Verdict == judge.CE {
			tgcomp.Text(c, "編譯錯誤")
		} else {
			tgcomp.Text(c, "stderr")
		}
		tgcomp.Code(c, cut(f.Stderr), &tgcomp.CodeConf{Language: "text"})
	}
}
