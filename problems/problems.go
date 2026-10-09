// Package problems parses problem sets.
//
// Each problem is a directory:
//
//	problems.json       every problem.json in one list, made by MakeList
//	<id>/problem.json   {"title": "...", "time_limit_ms": 1000, "time_limits_ms": {"cpp": 500},
//	                     "version": "2026-10-08 15:04:05", "tags": ["..."],
//	                     "solution_tags": ["..."], "compare": "float-diff 1e-6",
//	                     "subtasks": [{"score": 40, "tests": ["0[1-3]"], "constraints": "..."}]}
//	<id>/statement.md   a "## 提示" section becomes Hint
//	<id>/checker.js     optional; judges outputs instead of an exact match
//	<id>/interactor.js  optional; makes the problem interactive, .out optional
//	<id>/grader/<lang>/grader.<lang>    optional; the main program, which calls
//	                                    the submission's functions (lang: py, cpp, js, go)
//	<id>/grader/<lang>/template.<lang>  optional; the starting code, with a grader
//	<id>/tests/<name>.in, <name>.out   names starting with "sample" are shown
package problems

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
)

// Problem is one problem with its tests.
type Problem struct {
	ID        string
	Title     string
	Statement string
	Hint      string // markdown, shown collapsed; empty if none
	TimeLimit time.Duration
	// Per-language overrides of TimeLimit, keyed by language id ("py", "cpp", "js", "go").
	TimeLimits map[string]time.Duration
	Version    string // date of the last change, e.g. "2026-10-08 15:04:05"; may be empty
	Tags       []string
	// SolutionTags hint at the solution, so they are hidden by default.
	SolutionTags []string
	Cases        []judge.Case
	Compare      judge.Compare // built-in comparison, used without Checker
	Checker      string        // checker.js source; empty for Compare
	// Interactor is interactor.js source; empty unless interactive. Then
	// each case's Input is the interactor's input.
	Interactor string
	// Graders are grader sources by language id; empty unless the
	// submission is functions they call. Languages without one can't submit.
	Graders map[string]string
	// Templates are starting code by language id; may be empty.
	Templates map[string]string
	Subtasks  []Subtask // empty: all or nothing
	// Unsupported says why the problem can't be judged here; "" if it can.
	Unsupported string
}

// Subtask is a scored group of cases.
type Subtask struct {
	judge.Subtask
	Constraints string // markdown; may be empty
}

// JudgeSubtasks returns the subtasks for judge.Spec.
func (p *Problem) JudgeSubtasks() []judge.Subtask {
	var out []judge.Subtask
	for _, st := range p.Subtasks {
		out = append(out, st.Subtask)
	}
	return out
}

// TimeLimitFor returns the time limit of language lang.
func (p *Problem) TimeLimitFor(lang string) time.Duration {
	if t, ok := p.TimeLimits[lang]; ok {
		return t
	}
	return p.TimeLimit
}

// Samples returns the cases shown in the statement.
func (p *Problem) Samples() []judge.Case {
	var out []judge.Case
	for _, c := range p.Cases {
		if strings.HasPrefix(c.Name, "sample") {
			out = append(out, c)
		}
	}
	return out
}

// Meta is what problem.json holds.
type Meta struct {
	Title      string
	TimeLimit  time.Duration
	TimeLimits map[string]time.Duration
	Version    string
	Tags       []string
	// SolutionTags hint at the solution, so they are hidden by default.
	SolutionTags []string
	Compare      judge.Compare
	Subtasks     []SubtaskSpec
	// Unsupported says why the problem can't be judged here; "" if it can.
	// Not in problem.json: other formats set it.
	Unsupported string
}

// SubtaskSpec is a subtask in problem.json.
type SubtaskSpec struct {
	Score float64 `json:"score"`
	// Tests are test names or path.Match patterns, e.g. "01" or "1-*".
	Tests       []string `json:"tests"`
	Constraints string   `json:"constraints,omitempty"`
}

type meta struct {
	Title        string         `json:"title"`
	TimeLimitMS  int            `json:"time_limit_ms"`
	TimeLimitsMS map[string]int `json:"time_limits_ms,omitempty"`
	Version      string         `json:"version,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
	SolutionTags []string       `json:"solution_tags,omitempty"`
	Compare      string         `json:"compare,omitempty"`
	Subtasks     []SubtaskSpec  `json:"subtasks,omitempty"`
}

// ParseMeta parses problem.json.
func ParseMeta(bs []byte) (Meta, error) {
	var m meta
	if err := json.Unmarshal(bs, &m); err != nil {
		return Meta{}, err
	}
	return m.parse()
}

func (m meta) parse() (Meta, error) {
	if m.TimeLimitMS <= 0 {
		m.TimeLimitMS = 1000
	}
	var limits map[string]time.Duration
	for lang, ms := range m.TimeLimitsMS {
		if ms <= 0 {
			continue
		}
		if limits == nil {
			limits = map[string]time.Duration{}
		}
		limits[lang] = time.Duration(ms) * time.Millisecond
	}
	cmp, err := judge.ParseCompare(m.Compare)
	if err != nil {
		return Meta{}, err
	}
	for i, st := range m.Subtasks {
		if !(st.Score > 0) || len(st.Tests) == 0 {
			return Meta{}, fmt.Errorf("子任務 %d 要有正的 score 與 tests", i+1)
		}
		for _, pat := range st.Tests {
			if _, err := path.Match(pat, ""); err != nil {
				return Meta{}, fmt.Errorf("子任務 %d 的 tests 有誤：%q", i+1, pat)
			}
		}
	}
	return Meta{
		Title:        m.Title,
		TimeLimit:    time.Duration(m.TimeLimitMS) * time.Millisecond,
		TimeLimits:   limits,
		Version:      m.Version,
		Tags:         m.Tags,
		SolutionTags: m.SolutionTags,
		Compare:      cmp,
		Subtasks:     m.Subtasks,
	}, nil
}

// Entry is a problem in problems.json.
type Entry struct {
	ID string
	Meta
}

type entry struct {
	ID string `json:"id"`
	meta
}

// MakeList builds problems.json from every <id>/problem.json in fsys.
func MakeList(fsys fs.FS) ([]byte, error) {
	dirs, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	list := []entry{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		bs, err := fs.ReadFile(fsys, path.Join(d.Name(), "problem.json"))
		if err != nil {
			return nil, err
		}
		var m meta
		if err := json.Unmarshal(bs, &m); err != nil {
			return nil, fmt.Errorf("%s/problem.json: %w", d.Name(), err)
		}
		if _, err := m.parse(); err != nil {
			return nil, fmt.Errorf("%s/problem.json: %w", d.Name(), err)
		}
		list = append(list, entry{ID: d.Name(), meta: m})
	}
	bs, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(bs, '\n'), nil
}

// ParseList parses problems.json.
func ParseList(bs []byte) ([]Entry, error) {
	var list []entry
	if err := json.Unmarshal(bs, &list); err != nil {
		return nil, err
	}
	out := make([]Entry, len(list))
	for i, e := range list {
		if e.ID == "" || strings.Contains(e.ID, "/") {
			return nil, fmt.Errorf("題目 id 有誤：%q", e.ID)
		}
		m, err := e.meta.parse()
		if err != nil {
			return nil, fmt.Errorf("題目 %s：%w", e.ID, err)
		}
		out[i] = Entry{ID: e.ID, Meta: m}
	}
	return out, nil
}

// Load reads every problem in fsys, sorted by id.
func Load(fsys fs.FS) ([]*Problem, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}

	var ps []*Problem
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := LoadOne(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	return ps, nil
}

// LoadOne reads the problem in directory id of fsys.
func LoadOne(fsys fs.FS, id string) (*Problem, error) {
	bs, err := fs.ReadFile(fsys, path.Join(id, "problem.json"))
	if err != nil {
		return nil, fmt.Errorf("problem %s: %w", id, err)
	}
	m, err := ParseMeta(bs)
	if err != nil {
		return nil, fmt.Errorf("problem %s: %w", id, err)
	}
	return LoadWithMeta(fsys, id, m)
}

// LoadWithMeta reads the problem in directory id of fsys, without its
// problem.json.
func LoadWithMeta(fsys fs.FS, id string, m Meta) (*Problem, error) {
	p, err := loadOne(fsys, id, m)
	if err != nil {
		return nil, fmt.Errorf("problem %s: %w", id, err)
	}
	return p, nil
}

func loadOne(fsys fs.FS, id string, m Meta) (*Problem, error) {
	st, err := fs.ReadFile(fsys, path.Join(id, "statement.md"))
	if err != nil {
		return nil, err
	}

	checker, err := readOptional(fsys, path.Join(id, CheckerFile))
	if err != nil {
		return nil, err
	}
	interactor, err := readOptional(fsys, path.Join(id, InteractorFile))
	if err != nil {
		return nil, err
	}
	if checker != "" && interactor != "" {
		return nil, fmt.Errorf("%s 與 %s 只能有一個", CheckerFile, InteractorFile)
	}
	if !m.Compare.IsDefault() && (checker != "" || interactor != "") {
		return nil, fmt.Errorf("有 %s 或 %s 時不能設定 compare", CheckerFile, InteractorFile)
	}

	graders, templates, err := loadGraders(fsys, id)
	if err != nil {
		return nil, err
	}

	cases, err := loadCases(fsys, path.Join(id, "tests"), interactor != "")
	if err != nil {
		return nil, err
	}

	subtasks, err := resolveSubtasks(m.Subtasks, cases)
	if err != nil {
		return nil, err
	}

	stmt, hint := splitHint(string(st))
	return &Problem{
		ID:           id,
		Title:        m.Title,
		Statement:    stmt,
		Hint:         hint,
		TimeLimit:    m.TimeLimit,
		TimeLimits:   m.TimeLimits,
		Version:      m.Version,
		Tags:         m.Tags,
		SolutionTags: m.SolutionTags,
		Cases:        cases,
		Compare:      m.Compare,
		Checker:      checker,
		Interactor:   interactor,
		Graders:      graders,
		Templates:    templates,
		Subtasks:     subtasks,
	}, nil
}

// resolveSubtasks matches specs' tests against cases. Every pattern must
// match a case, and every case but samples must be in a subtask.
func resolveSubtasks(specs []SubtaskSpec, cases []judge.Case) ([]Subtask, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	used := map[string]bool{}
	var out []Subtask
	for i, sp := range specs {
		st := Subtask{Subtask: judge.Subtask{Score: sp.Score}, Constraints: sp.Constraints}
		for _, pat := range sp.Tests {
			n := 0
			for _, c := range cases {
				if ok, _ := path.Match(pat, c.Name); ok {
					n++
					used[c.Name] = true
					if !slices.Contains(st.Cases, c.Name) {
						st.Cases = append(st.Cases, c.Name)
					}
				}
			}
			if n == 0 {
				return nil, fmt.Errorf("子任務 %d 的 %q 沒有符合的測資", i+1, pat)
			}
		}
		out = append(out, st)
	}
	for _, c := range cases {
		if !used[c.Name] && !strings.HasPrefix(c.Name, "sample") {
			return nil, fmt.Errorf("測資 %s 不屬於任何子任務", c.Name)
		}
	}
	return out, nil
}

// CheckerFile and InteractorFile are optional files of a problem.
const (
	CheckerFile    = "checker.js"
	InteractorFile = "interactor.js"
)

// Langs are the language ids of graders and templates.
var Langs = []string{"py", "cpp", "js", "go"}

// GraderFile and TemplateFile are the paths of lang's grader and template in
// a problem. One directory per language, so Go and C++ files don't mix.
func GraderFile(lang string) string   { return "grader/" + lang + "/grader." + lang }
func TemplateFile(lang string) string { return "grader/" + lang + "/template." + lang }

// loadGraders reads the graders and templates of problem id.
func loadGraders(fsys fs.FS, id string) (graders, templates map[string]string, err error) {
	for _, lang := range Langs {
		g, err := readOptional(fsys, path.Join(id, GraderFile(lang)))
		if err != nil {
			return nil, nil, err
		}
		t, err := readOptional(fsys, path.Join(id, TemplateFile(lang)))
		if err != nil {
			return nil, nil, err
		}
		if t != "" && g == "" {
			return nil, nil, fmt.Errorf("有 %s 但沒有 %s", TemplateFile(lang), GraderFile(lang))
		}
		if g != "" {
			if graders == nil {
				graders, templates = map[string]string{}, map[string]string{}
			}
			graders[lang] = g
			if t != "" {
				templates[lang] = t
			}
		}
	}
	return graders, templates, nil
}

// readOptional reads name, or returns "" if it doesn't exist.
func readOptional(fsys fs.FS, name string) (string, error) {
	bs, err := fs.ReadFile(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(bs), err
}

const hintHeading = "## 提示"

// splitHint moves the hint section, up to the next "## " heading, out of st.
func splitHint(st string) (stmt, hint string) {
	lines := strings.SplitAfter(st, "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == hintHeading {
			start = i
			break
		}
	}
	if start < 0 {
		return st, ""
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	hint = strings.TrimSpace(strings.Join(lines[start+1:end], ""))
	stmt = strings.Join(lines[:start], "") + strings.Join(lines[end:], "")
	return strings.TrimRight(stmt, "\n") + "\n", hint
}

// loadCases reads the tests in dir; with optionalOut a missing .out is "".
func loadCases(fsys fs.FS, dir string, optionalOut bool) ([]judge.Case, error) {
	ins, err := fs.Glob(fsys, path.Join(dir, "*.in"))
	if err != nil {
		return nil, err
	}
	if len(ins) == 0 {
		return nil, fmt.Errorf("no tests in %s", dir)
	}
	// Samples first, then by name.
	sort.Slice(ins, func(i, j int) bool {
		si := strings.HasPrefix(path.Base(ins[i]), "sample")
		sj := strings.HasPrefix(path.Base(ins[j]), "sample")
		if si != sj {
			return si
		}
		return ins[i] < ins[j]
	})

	cases := make([]judge.Case, 0, len(ins))
	for _, in := range ins {
		name := strings.TrimSuffix(path.Base(in), ".in")
		inBs, err := fs.ReadFile(fsys, in)
		if err != nil {
			return nil, err
		}
		outBs, err := fs.ReadFile(fsys, strings.TrimSuffix(in, ".in")+".out")
		if err != nil && !(optionalOut && errors.Is(err, fs.ErrNotExist)) {
			return nil, err
		}
		cases = append(cases, judge.Case{
			Name:   name,
			Input:  string(inBs),
			Output: string(outBs),
		})
	}
	return cases, nil
}
