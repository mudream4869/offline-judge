package problems

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mudream4869/offline-judge/internal/judge"
)

// Kattis problem packages (https://www.kattis.com/problem-package-format/),
// legacy and 2023-07+:
//
//	problem.yaml                          name, limits, validation, keywords
//	statement/ or problem_statement/      problem.<lang>.md or .tex
//	data/sample/**.in, .ans               shown in the statement
//	data/secret/**.in, .ans               scored groups are subdirectories
//	data/**/testdata.yaml, test_group.yaml
//
// Custom output validators, interactive problems and include/ need programs
// this judge can't run; such problems load with Unsupported set.

// KattisMetaFile is the file that makes a directory a Kattis package.
const KattisMetaFile = "problem.yaml"

// KattisDefaultTimeLimit is used when problem.yaml has no time limit, as in
// the legacy format, where judges infer it from the example submissions.
const KattisDefaultTimeLimit = 2 * time.Second

// Languages of statements and names, most preferred first.
var kattisLangs = []string{"zh-TW", "zh-tw", "zh-Hant", "zh", "en", ""}

type kattisYAML struct {
	Type           any    `yaml:"type"` // a string, or a list from 2023-07
	Name           any    `yaml:"name"` // a string, or a map by language
	Keywords       any    `yaml:"keywords"`
	Validation     string `yaml:"validation"` // legacy
	ValidatorFlags string `yaml:"validator_flags"`
	Limits         struct {
		TimeLimit float64 `yaml:"time_limit"` // seconds, 2023-07+
	} `yaml:"limits"`
}

// ParseKattisMeta parses problem.yaml; files are the package's paths,
// relative to its directory, to spot what can't be judged here.
func ParseKattisMeta(bs []byte, files []string) (Meta, error) {
	var y kattisYAML
	if err := yaml.Unmarshal(bs, &y); err != nil {
		return Meta{}, fmt.Errorf("%s：%w", KattisMetaFile, err)
	}
	m := Meta{
		Title:     pickLang(stringMap(y.Name)),
		TimeLimit: KattisDefaultTimeLimit,
		Tags:      stringList(y.Keywords),
	}
	if y.Limits.TimeLimit > 0 {
		m.TimeLimit = time.Duration(y.Limits.TimeLimit * float64(time.Second))
	}
	cmp, err := judge.ParseCompare("tokens " + y.ValidatorFlags)
	if err != nil {
		return Meta{}, fmt.Errorf("%s 的 validator_flags：%w", KattisMetaFile, err)
	}
	m.Compare = cmp

	types := stringList(y.Type)
	validation := strings.Fields(y.Validation)
	hasDir := func(dir string) bool {
		return slices.ContainsFunc(files, func(f string) bool { return strings.HasPrefix(f, dir+"/") })
	}
	switch {
	case slices.Contains(types, "interactive") || slices.Contains(validation, "interactive"):
		m.Unsupported = "互動題需要 Kattis 的互動驗證器（程式），這裡還不能執行"
	case slices.Contains(types, "multi-pass") || slices.Contains(types, "submit-answer"):
		m.Unsupported = "還不支援這種題型（" + strings.Join(types, "、") + "）"
	case slices.Contains(validation, "custom") || hasDir("output_validators") || hasDir("output_validator"):
		m.Unsupported = "這題用自訂的輸出驗證器（程式），這裡還不能執行"
	case hasDir("include"):
		m.Unsupported = "這題要和 include/ 的程式碼一起編譯，這裡還不支援"
	}
	return m, nil
}

// isKattisScoring reports whether problem.yaml marks a scoring problem.
func isKattisScoring(bs []byte) bool {
	var y kattisYAML
	if yaml.Unmarshal(bs, &y) != nil {
		return false
	}
	return slices.Contains(stringList(y.Type), "scoring")
}

// LoadKattis reads the Kattis package in directory id of fsys.
func LoadKattis(fsys fs.FS, id string, m Meta) (*Problem, error) {
	p, err := loadKattis(fsys, id, m)
	if err != nil {
		return nil, fmt.Errorf("problem %s: %w", id, err)
	}
	return p, nil
}

func loadKattis(fsys fs.FS, id string, m Meta) (*Problem, error) {
	yml, err := fs.ReadFile(fsys, path.Join(id, KattisMetaFile))
	if err != nil {
		return nil, err
	}
	title := m.Title
	if title == "" {
		title = id
	}
	stmt, err := kattisStatement(fsys, id)
	if err != nil {
		return nil, err
	}

	p := &Problem{
		ID:          id,
		Title:       title,
		TimeLimit:   m.TimeLimit,
		Version:     m.Version,
		Tags:        m.Tags,
		Compare:     m.Compare,
		Unsupported: m.Unsupported,
	}
	p.Cases, err = kattisCases(fsys, id, m.Unsupported != "")
	if err != nil {
		return nil, err
	}
	if len(p.Cases) == 0 {
		return nil, fmt.Errorf("data/ 裡沒有測資")
	}
	if flags := kattisValidatorFlags(fsys, id); flags != "" {
		cmp, err := judge.ParseCompare(m.Compare.String() + " " + flags)
		if err != nil {
			return nil, fmt.Errorf("output_validator 參數：%w", err)
		}
		p.Compare = cmp
	}

	var note string
	if isKattisScoring(yml) {
		p.Subtasks, note = kattisSubtasks(fsys, id, p.Cases)
	}
	p.Statement = "# " + title + "\n\n" + stmt
	if note != "" {
		p.Statement += "\n\n> " + note + "\n"
	}
	return p, nil
}

// kattisStatement returns the statement as Markdown, preferring Markdown
// over LaTeX and kattisLangs in order.
func kattisStatement(fsys fs.FS, id string) (string, error) {
	var files []string
	for _, dir := range []string{"statement", "problem_statement"} {
		ms, _ := fs.Glob(fsys, path.Join(id, dir, "problem*"))
		files = append(files, ms...)
	}
	// By language, then Markdown before LaTeX.
	byLang := map[string]map[string]string{}
	for _, f := range files {
		base := path.Base(f)
		ext := path.Ext(base)
		if ext != ".md" && ext != ".tex" {
			continue
		}
		lang := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSuffix(base, ext), "problem"), ".")
		if byLang[lang] == nil {
			byLang[lang] = map[string]string{}
		}
		byLang[lang][ext] = f
	}
	if len(byLang) == 0 {
		return "", fmt.Errorf("找不到題目敘述（statement/problem.<語言>.md 或 .tex）")
	}
	byExt := byLang[pickLangKey(byLang)]
	f, ok := byExt[".md"]
	if !ok {
		f = byExt[".tex"]
	}
	bs, err := fs.ReadFile(fsys, f)
	if err != nil {
		return "", err
	}
	if path.Ext(f) == ".tex" {
		return texToMarkdown(string(bs)), nil
	}
	return string(bs), nil
}

// kattisCases reads data/sample and data/secret; names keep their path
// under data/, so samples start with "sample". With optionalAns a missing
// .ans is "" (interactive problems may have none).
func kattisCases(fsys fs.FS, id string, optionalAns bool) ([]judge.Case, error) {
	var cases []judge.Case
	for _, group := range []string{"sample", "secret"} {
		root := path.Join(id, "data", group)
		var ins []string
		err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".in") {
				ins = append(ins, p)
			}
			return nil
		})
		if err != nil && !(group == "sample" && errors.Is(err, fs.ErrNotExist)) {
			return nil, err
		}
		sort.Strings(ins)
		for _, in := range ins {
			name := strings.TrimSuffix(strings.TrimPrefix(in, path.Join(id, "data")+"/"), ".in")
			inBs, err := fs.ReadFile(fsys, in)
			if err != nil {
				return nil, err
			}
			ans, err := fs.ReadFile(fsys, strings.TrimSuffix(in, ".in")+".ans")
			if err != nil && !(optionalAns && errors.Is(err, fs.ErrNotExist)) {
				return nil, err
			}
			cases = append(cases, judge.Case{Name: name, Input: string(inBs), Output: string(ans)})
		}
	}
	return cases, nil
}

// kattisValidatorFlags returns the default validator flags set for the
// secret data: output_validator_flags (legacy testdata.yaml) or
// output_validator_args (test_group.yaml), nearest first.
func kattisValidatorFlags(fsys fs.FS, id string) string {
	for _, dir := range []string{"data/secret", "data"} {
		for _, name := range []string{"test_group.yaml", "testdata.yaml"} {
			bs, err := fs.ReadFile(fsys, path.Join(id, dir, name))
			if err != nil {
				continue
			}
			var y struct {
				Flags any `yaml:"output_validator_flags"`
				Args  any `yaml:"output_validator_args"`
			}
			if yaml.Unmarshal(bs, &y) != nil {
				continue
			}
			for _, v := range []any{y.Args, y.Flags} {
				if l := stringList(v); len(l) > 0 {
					return strings.Join(l, " ")
				}
			}
		}
	}
	return ""
}

// kattisGroup is the scoring of a group in testdata.yaml or test_group.yaml.
type kattisGroup struct {
	// 2023-07+
	MaxScore    any `yaml:"max_score"`
	Aggregation any `yaml:"score_aggregation"`
	RequirePass any `yaml:"require_pass"`
	// legacy
	AcceptScore *float64 `yaml:"accept_score"`
	Range       string   `yaml:"range"`
	GraderFlags string   `yaml:"grader_flags"`
}

// kattisSubtasks turns the groups of data/secret into subtasks. A group
// maps when it scores all or nothing; otherwise it returns no subtasks and
// a note that the problem is judged pass-fail.
func kattisSubtasks(fsys fs.FS, id string, cases []judge.Case) ([]Subtask, string) {
	const fallback = "原題依測資部分給分，這裡只判斷是否全部通過。"
	entries, err := fs.ReadDir(fsys, path.Join(id, "data", "secret"))
	if err != nil {
		return nil, fallback
	}
	var subs []Subtask
	byName := map[string]int{}
	var requires [][]string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		prefix := "secret/" + e.Name() + "/"
		var names []string
		for _, c := range cases {
			if strings.HasPrefix(c.Name, prefix) {
				names = append(names, c.Name)
			}
		}
		if len(names) == 0 {
			continue
		}
		g, ok := readKattisGroup(fsys, path.Join(id, "data", "secret", e.Name()))
		if !ok {
			return nil, fallback
		}
		score, ok := g.allOrNothing()
		if !ok {
			return nil, fallback
		}
		byName["secret/"+e.Name()] = len(subs)
		subs = append(subs, Subtask{Subtask: judge.Subtask{Score: score, Cases: names},
			Constraints: e.Name()})
		requires = append(requires, stringList(g.RequirePass))
	}
	// Loose tests directly in data/secret belong to no group.
	for _, c := range cases {
		if strings.HasPrefix(c.Name, "secret/") && strings.Count(c.Name, "/") == 1 {
			return nil, fallback
		}
	}
	if len(subs) == 0 {
		return nil, fallback
	}
	// require_pass: passing the required groups is part of the subtask.
	for i, req := range requires {
		for _, r := range req {
			j, ok := byName[r]
			if !ok {
				return nil, fallback
			}
			for _, n := range subs[j].Cases {
				if !slices.Contains(subs[i].Cases, n) {
					subs[i].Cases = append(subs[i].Cases, n)
				}
			}
		}
	}
	return subs, ""
}

func readKattisGroup(fsys fs.FS, dir string) (kattisGroup, bool) {
	for _, name := range []string{"test_group.yaml", "testdata.yaml"} {
		bs, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			continue
		}
		var g kattisGroup
		return g, yaml.Unmarshal(bs, &g) == nil
	}
	return kattisGroup{}, false
}

// allOrNothing returns the group's score if it is all or nothing.
func (g kattisGroup) allOrNothing() (float64, bool) {
	if g.MaxScore != nil { // 2023-07+: groups are pass-fail unless aggregated
		score, err := strconv.ParseFloat(fmt.Sprint(g.MaxScore), 64)
		agg := fmt.Sprint(g.Aggregation)
		return score, err == nil && score > 0 && (g.Aggregation == nil || agg == "pass-fail" || agg == "min")
	}
	// Legacy: the min grader with every accepted test worth the group's
	// score; a failed test scores 0, so the group scores all or nothing.
	if !slices.Contains(strings.Fields(g.GraderFlags), "min") || g.AcceptScore == nil {
		return 0, false
	}
	score := *g.AcceptScore
	if f := strings.Fields(g.Range); len(f) == 2 {
		if hi, err := strconv.ParseFloat(f[1], 64); err == nil {
			score = min(score, hi)
		}
	}
	return score, score > 0
}

// stringMap reads a string, or a map of strings, keyed "" for a string.
func stringMap(v any) map[string]string {
	switch v := v.(type) {
	case string:
		return map[string]string{"": v}
	case map[string]any:
		out := map[string]string{}
		for k, x := range v {
			if s, ok := x.(string); ok {
				out[k] = s
			}
		}
		return out
	}
	return nil
}

// stringList reads a space-separated string or a list of strings.
func stringList(v any) []string {
	switch v := v.(type) {
	case string:
		return strings.Fields(v)
	case []any:
		var out []string
		for _, x := range v {
			out = append(out, fmt.Sprint(x))
		}
		return out
	}
	return nil
}

// pickLang returns the value of the most preferred language in m.
func pickLang(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	return m[pickLangKey(m)]
}

func pickLangKey[V any](m map[string]V) string {
	for _, l := range kattisLangs {
		if _, ok := m[l]; ok {
			return l
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[0]
}
