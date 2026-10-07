// Package problems parses problem sets.
//
// Each problem is a directory:
//
//	problems.json       every problem.json in one list, made by MakeList
//	<id>/problem.json   {"title": "...", "time_limit_ms": 1000}
//	<id>/statement.md
//	<id>/tests/<name>.in, <name>.out   names starting with "sample" are shown
package problems

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
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
	TimeLimit time.Duration
	Cases     []judge.Case
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
	Title     string
	TimeLimit time.Duration
}

type meta struct {
	Title       string `json:"title"`
	TimeLimitMS int    `json:"time_limit_ms"`
}

// ParseMeta parses problem.json.
func ParseMeta(bs []byte) (Meta, error) {
	var m meta
	if err := json.Unmarshal(bs, &m); err != nil {
		return Meta{}, err
	}
	return m.parse(), nil
}

func (m meta) parse() Meta {
	if m.TimeLimitMS <= 0 {
		m.TimeLimitMS = 1000
	}
	return Meta{
		Title:     m.Title,
		TimeLimit: time.Duration(m.TimeLimitMS) * time.Millisecond,
	}
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
		out[i] = Entry{ID: e.ID, Meta: e.meta.parse()}
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

	cases, err := loadCases(fsys, path.Join(id, "tests"))
	if err != nil {
		return nil, err
	}

	return &Problem{
		ID:        id,
		Title:     m.Title,
		Statement: string(st),
		TimeLimit: m.TimeLimit,
		Cases:     cases,
	}, nil
}

func loadCases(fsys fs.FS, dir string) ([]judge.Case, error) {
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
		if err != nil {
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
