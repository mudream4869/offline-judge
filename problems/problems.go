// Package problems embeds the problem set.
//
// Each problem is a directory:
//
//	<id>/problem.json   {"title": "...", "time_limit_ms": 1000}
//	<id>/statement.md
//	<id>/tests/<name>.in, <name>.out   names starting with "sample" are shown
package problems

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
)

//go:embed */problem.json */statement.md */tests
var data embed.FS

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

type meta struct {
	Title       string `json:"title"`
	TimeLimitMS int    `json:"time_limit_ms"`
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
		p, err := loadOne(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("problem %s: %w", e.Name(), err)
		}
		ps = append(ps, p)
	}
	return ps, nil
}

// All loads the embedded problems.
func All() ([]*Problem, error) {
	return Load(data)
}

func loadOne(fsys fs.FS, id string) (*Problem, error) {
	bs, err := fs.ReadFile(fsys, path.Join(id, "problem.json"))
	if err != nil {
		return nil, err
	}
	var m meta
	if err := json.Unmarshal(bs, &m); err != nil {
		return nil, err
	}
	if m.TimeLimitMS <= 0 {
		m.TimeLimitMS = 1000
	}

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
		TimeLimit: time.Duration(m.TimeLimitMS) * time.Millisecond,
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
