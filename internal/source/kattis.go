package source

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/mudream4869/offline-judge/problems"
)

// kattis is a directory of Kattis problem packages, <id>/problem.yaml,
// as contests publish them; there is no list file, so every problem.yaml
// comes with the list.
type kattis struct{}

// kattisIDs returns the packages in ix, sorted.
func kattisIDs(ix *Index) []string {
	var ids []string
	for _, f := range ix.Files {
		id, ok := strings.CutSuffix(f.Path, "/"+problems.KattisMetaFile)
		if ok && !strings.Contains(id, "/") {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (kattis) listFiles(ix *Index) ([]File, error) {
	var out []File
	for _, id := range kattisIDs(ix) {
		f, _ := ix.find(id + "/" + problems.KattisMetaFile)
		out = append(out, f)
	}
	return out, nil
}

func (k kattis) list(ix *Index, read func(File) ([]byte, error)) ([]problems.Entry, error) {
	var out []problems.Entry
	for _, id := range kattisIDs(ix) {
		f, _ := ix.find(id + "/" + problems.KattisMetaFile)
		bs, err := read(f)
		if err != nil {
			return nil, err
		}
		var rel []string
		for _, f := range ix.Files {
			if r, ok := strings.CutPrefix(f.Path, id+"/"); ok {
				rel = append(rel, r)
			}
		}
		m, err := problems.ParseKattisMeta(bs, rel)
		if err != nil {
			return nil, fmt.Errorf("%s：%w", id, err)
		}
		if m.Title == "" {
			m.Title = id
		}
		m.Version = kattisVersion(k.files(ix, id))
		out = append(out, problems.Entry{ID: id, Meta: m})
	}
	return out, nil
}

func (kattis) files(ix *Index, id string) []File {
	var out []File
	for _, f := range ix.Files {
		rest, ok := strings.CutPrefix(f.Path, id+"/")
		if !ok {
			continue
		}
		dir, base := path.Dir(rest), path.Base(rest)
		switch {
		case rest == problems.KattisMetaFile:
		case (dir == "statement" || dir == "problem_statement") && strings.HasPrefix(base, "problem") &&
			(path.Ext(base) == ".md" || path.Ext(base) == ".tex"):
		case strings.HasPrefix(rest, "data/") && (path.Ext(base) == ".in" || path.Ext(base) == ".ans" ||
			base == "testdata.yaml" || base == "test_group.yaml"):
		default:
			continue
		}
		out = append(out, f)
	}
	return out
}

func (kattis) load(fsys fs.FS, id string, m problems.Meta) (*problems.Problem, error) {
	return problems.LoadKattis(fsys, id, m)
}

// kattisVersion names the content of files, so a change marks old
// submissions; packages have no version of their own.
func kattisVersion(files []File) string {
	lines := make([]string, len(files))
	for i, f := range files {
		lines[i] = f.Path + " " + f.SHA
	}
	sort.Strings(lines)
	h := sha1.Sum([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:6])
}
