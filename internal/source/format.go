package source

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/mudream4869/offline-judge/problems"
)

// format is a layout of problems in a source.
type format interface {
	// listFiles returns the files the problem list is built from.
	listFiles(ix *Index) ([]File, error)
	// list builds the problem list; read returns one of listFiles.
	list(ix *Index, read func(File) ([]byte, error)) ([]problems.Entry, error)
	// files returns what problem id needs.
	files(ix *Index, id string) []File
	// load reads problem id from fsys, which holds its files.
	load(fsys fs.FS, id string, m problems.Meta) (*problems.Problem, error)
}

// formatOf returns the format of the source listed by ix.
func formatOf(ix *Index) (format, error) {
	if _, ok := ix.find(listFile); ok {
		return native{}, nil
	}
	if len(kattisIDs(ix)) > 0 {
		return kattis{}, nil
	}
	return nil, fmt.Errorf("來源缺少 %s，也沒有 Kattis 題目包（<題目>/%s）", listFile, problems.KattisMetaFile)
}

// listFile lists the problems of a source, so the list is one download.
const listFile = "problems.json"

// native is this repo's format: problems.json and <id>/ (see package problems).
type native struct{}

func (native) listFiles(ix *Index) ([]File, error) {
	f, ok := ix.find(listFile)
	if !ok {
		return nil, fmt.Errorf("來源缺少 %s", listFile)
	}
	return []File{f}, nil
}

func (native) list(ix *Index, read func(File) ([]byte, error)) ([]problems.Entry, error) {
	f, _ := ix.find(listFile)
	bs, err := read(f)
	if err != nil {
		return nil, err
	}
	list, err := problems.ParseList(bs)
	if err != nil {
		return nil, fmt.Errorf("%s：%w", listFile, err)
	}
	return list, nil
}

func (native) files(ix *Index, id string) []File {
	var out []File
	for _, f := range ix.Files {
		rest, ok := strings.CutPrefix(f.Path, id+"/")
		if !ok {
			continue
		}
		if rest == "statement.md" || rest == problems.CheckerFile || rest == problems.InteractorFile ||
			(path.Dir(rest) == "tests" && (path.Ext(rest) == ".in" || path.Ext(rest) == ".out")) {
			out = append(out, f)
		}
	}
	return out
}

func (native) load(fsys fs.FS, id string, m problems.Meta) (*problems.Problem, error) {
	return problems.LoadWithMeta(fsys, id, m)
}

// find returns the file at p.
func (ix *Index) find(p string) (File, bool) {
	for _, f := range ix.Files {
		if f.Path == p {
			return f, true
		}
	}
	return File{}, false
}
