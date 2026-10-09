package problems

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// kattisFS builds a package "p" from path → content.
func kattisFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for p, s := range files {
		fsys["p/"+p] = &fstest.MapFile{Data: []byte(s)}
	}
	return fsys
}

func loadKattisFS(t *testing.T, files map[string]string) *Problem {
	t.Helper()
	fsys := kattisFS(files)
	var rel []string
	for p := range files {
		rel = append(rel, p)
	}
	m, err := ParseKattisMeta([]byte(files["problem.yaml"]), rel)
	if err != nil {
		t.Fatal(err)
	}
	p, err := LoadKattis(fsys, "p", "p", m)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestKattisLegacy(t *testing.T) {
	p := loadKattisFS(t, map[string]string{
		"problem.yaml": "name: A Different Problem\nsource: Kattis\n" +
			"validator_flags: float_tolerance 1e-6\nkeywords: math easy\n",
		"problem_statement/problem.en.tex": "\\problemname{A Different Problem}\n" +
			"Compute $|a-b|$.\n\\section*{Input}\nTwo integers.\n% a comment\n",
		"data/sample/1.in":      "10 12\n",
		"data/sample/1.ans":     "2\n",
		"data/secret/01.in":     "1 1\n",
		"data/secret/01.ans":    "0\n",
		"data/secret/01.desc":   "ignored",
		"submissions/a/x.cpp":   "ignored",
		"input_validators/v.py": "ignored",
	})
	if p.Title != "A Different Problem" || p.TimeLimit != KattisDefaultTimeLimit || p.Unsupported != "" {
		t.Errorf("problem = %+v", p)
	}
	if !slices.Equal(p.Tags, []string{"math", "easy"}) {
		t.Errorf("tags = %v", p.Tags)
	}
	if got := p.Compare.String(); got != "tokens float_tolerance 1e-06" {
		t.Errorf("compare = %q", got)
	}
	want := "# A Different Problem\n\nCompute $|a-b|$.\n\n## Input\n\nTwo integers.\n"
	if p.Statement != want {
		t.Errorf("statement = %q, want %q", p.Statement, want)
	}
	var names []string
	for _, c := range p.Cases {
		names = append(names, c.Name)
	}
	if !slices.Equal(names, []string{"sample/1", "secret/01"}) || len(p.Samples()) != 1 ||
		p.Cases[0].Output != "2\n" || len(p.Subtasks) != 0 {
		t.Errorf("cases = %v, samples = %d, subtasks = %v", names, len(p.Samples()), p.Subtasks)
	}
}

func TestKattisLegacyScoring(t *testing.T) {
	group := "on_reject: break\naccept_score: 50\nrange: 0 50\ngrader_flags: min\n"
	p := loadKattisFS(t, map[string]string{
		"problem.yaml":                       "type: scoring\nname: Odd Echo\n",
		"statement/problem.en.md":            "Echo every other word.\n",
		"data/sample/1.in":                   "1\na\n",
		"data/sample/1.ans":                  "a\n",
		"data/secret/subtask1/testdata.yaml": group,
		"data/secret/subtask1/1.in":          "1\n",
		"data/secret/subtask1/1.ans":         "1\n",
		"data/secret/subtask2/testdata.yaml": group,
		"data/secret/subtask2/1.in":          "2\n",
		"data/secret/subtask2/1.ans":         "2\n",
		"data/secret/subtask2/2.in":          "3\n",
		"data/secret/subtask2/2.ans":         "3\n",
	})
	if len(p.Subtasks) != 2 || p.Subtasks[0].Score != 50 ||
		!slices.Equal(p.Subtasks[1].Cases, []string{"secret/subtask2/1", "secret/subtask2/2"}) {
		t.Fatalf("subtasks = %+v", p.Subtasks)
	}
	if strings.Contains(p.Statement, "只判斷是否全部通過") {
		t.Error("mapped groups shouldn't note a fallback")
	}

	// The default sum grader scores tests one by one: judged pass-fail, with a note.
	p = loadKattisFS(t, map[string]string{
		"problem.yaml":                 "type: scoring\n",
		"statement/problem.md":         "x\n",
		"data/secret/g1/testdata.yaml": "accept_score: 10\n",
		"data/secret/g1/1.in":          "1\n",
		"data/secret/g1/1.ans":         "1\n",
	})
	if len(p.Subtasks) != 0 || !strings.Contains(p.Statement, "只判斷是否全部通過") || p.Title != "p" {
		t.Errorf("subtasks = %+v, title %q, statement %q", p.Subtasks, p.Title, p.Statement)
	}
}

func TestKattis2023(t *testing.T) {
	p := loadKattisFS(t, map[string]string{
		"problem.yaml": "problem_format_version: 2023-07-draft\ntype: [pass-fail, scoring]\n" +
			"name:\n  en: Sum\n  zh: 總和\nkeywords: [math]\nlimits:\n  time_limit: 1.5\n",
		"statement/problem.en.md":       "English\n",
		"statement/problem.zh.md":       "中文\n",
		"statement/problem.en.tex":      "\\problemname{Sum} LaTeX\n",
		"data/secret/test_group.yaml":   "output_validator_args: [case_sensitive]\n",
		"data/secret/a/test_group.yaml": "max_score: 30\n",
		"data/secret/a/1.in":            "1\n",
		"data/secret/a/1.ans":           "1\n",
		"data/secret/b/test_group.yaml": "max_score: 70\nrequire_pass: secret/a\n",
		"data/secret/b/1.in":            "2\n",
		"data/secret/b/1.ans":           "2\n",
	})
	if p.Title != "總和" || p.TimeLimit != 1500*time.Millisecond || p.Statement != "# 總和\n\n中文\n" {
		t.Errorf("title %q, limit %v, statement %q", p.Title, p.TimeLimit, p.Statement)
	}
	if got := p.Compare.String(); got != "tokens case_sensitive" {
		t.Errorf("compare = %q", got)
	}
	if len(p.Subtasks) != 2 || p.Subtasks[1].Score != 70 ||
		!slices.Equal(p.Subtasks[1].Cases, []string{"secret/b/1", "secret/a/1"}) {
		t.Errorf("subtasks = %+v", p.Subtasks)
	}
}

func TestKattisUnsupported(t *testing.T) {
	for name, files := range map[string][]string{
		"validation: custom\n":                    nil,
		"validation: custom interactive\n":        nil,
		"type: interactive\n":                     nil,
		"type: submit-answer\n":                   nil,
		"name: x\n":                               {"output_validators/v/validate.cc"},
		"problem_format_version: 2023-07-draft\n": {"output_validator/validate.cc"},
		"name: y\n":                               {"include/cpp/main.cpp"},
	} {
		m, err := ParseKattisMeta([]byte(name), files)
		if err != nil || m.Unsupported == "" {
			t.Errorf("ParseKattisMeta(%q, %v) = %q, %v; want unsupported", name, files, m.Unsupported, err)
		}
	}
	if _, err := ParseKattisMeta([]byte("validator_flags: nope\n"), nil); err == nil {
		t.Error("bad validator_flags accepted")
	}
}

// The language comes first: English LaTeX beats Swedish Markdown.
func TestKattisStatementLanguage(t *testing.T) {
	p := loadKattisFS(t, map[string]string{
		"problem.yaml":             "name: Echo\n",
		"statement/problem.sv.md":  "Svenska\n",
		"statement/problem.en.tex": "\\problemname{Echo}\nEnglish\n",
		"data/secret/1.in":         "1\n",
		"data/secret/1.ans":        "1\n",
	})
	if p.Statement != "# Echo\n\nEnglish\n" {
		t.Errorf("statement = %q", p.Statement)
	}
}

// A package at the root of fsys, as a repo of one problem.
func TestKattisRoot(t *testing.T) {
	fsys := fstest.MapFS{
		"problem.yaml":            {Data: []byte("name: Two Sum\n")},
		"statement/problem.en.md": {Data: []byte("Add.\n")},
		"data/sample/1.in":        {Data: []byte("1 2\n")},
		"data/sample/1.ans":       {Data: []byte("3\n")},
		"data/secret/g/1.in":      {Data: []byte("2 2\n")},
		"data/secret/g/1.ans":     {Data: []byte("4\n")},
	}
	m, err := ParseKattisMeta(fsys["problem.yaml"].Data, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := LoadKattis(fsys, ".", "two-sum", m)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "two-sum" || p.Title != "Two Sum" || len(p.Cases) != 2 ||
		p.Cases[0].Name != "sample/1" || p.Cases[1].Name != "secret/g/1" {
		t.Errorf("problem = %+v", p)
	}
}

func TestTexToMarkdown(t *testing.T) {
	tex := "\\problemname{X}\n\\section*{Output}\n" +
		"Print ``yes'' or \\texttt{no}, \\textbf{bold} \\emph{it} 50\\% \\(a_i\\).\n" +
		"\\begin{itemize}\n\\item one\n\\item two\n\\end{itemize}\n" +
		"\\begin{tabular}{|l|l|}\n\\hline\nA & B \\\\ \\hline\n1 & 2 \\\\\n\\end{tabular}\n" +
		"\\illustration{0.3}{cave.jpg}{A cave}\n"
	want := "## Output\n\nPrint “yes” or `no`, **bold** *it* 50% $a_i$.\n\n" +
		"- one\n- two\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n（圖：A cave）\n"
	if got := texToMarkdown(tex); got != want {
		t.Errorf("texToMarkdown =\n%s\nwant\n%s", got, want)
	}
}
