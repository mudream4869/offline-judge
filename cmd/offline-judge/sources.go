//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"sync"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/source"
	"github.com/mudream4869/offline-judge/problems"
)

const defaultSource = "https://github.com/mudream4869/offline-judge/tree/main/problems"

// recommended are sources 設定 offers to add with one click.
var recommended = []struct{ name, url, note string }{
	{"Offline Judge 題庫", defaultSource, "本站的題目，含互動題與子任務"},
	{"Kattis problemtools 範例", "https://github.com/Kattis/problemtools/tree/master/examples",
		"Kattis 格式的 4 個範例題，其中 2 題需要自訂驗證器、無法評測"},
	{"CITS3001 Algorithms Project", "https://github.com/allanwupi/CITS3001_Algorithms_Project",
		"西澳大學演算法課的 5 題，Kattis 格式，英文"},
}

// sourceURLs returns the sources the user set, in order. Before there was a
// list, there was one source.
func sourceURLs() []string {
	if raw := memo.getText("sources", ""); raw != "" {
		var urls []string
		if json.Unmarshal([]byte(raw), &urls) == nil {
			return urls
		}
	}
	if old := memo.getText("source", ""); old != "" {
		return []string{old}
	}
	return []string{defaultSource}
}

func setSourceURLs(urls []string) {
	bs, _ := json.Marshal(urls)
	memo.setText("sources", string(bs))
}

// sourceLabel names a source: owner/repo[/dir], without the ref.
func sourceLabel(url string) string {
	r, err := source.Parse(url)
	if err != nil {
		return url
	}
	label := r.Owner + "/" + r.Name
	if r.Dir != "" {
		label += "/" + r.Dir
	}
	return label
}

// problemKey is the app-wide id of problem id in source url: problems of
// the default source keep their ids (and so their submissions and drafts);
// others are prefixed with the source.
func problemKey(url, id string) string {
	if url == defaultSource {
		return id
	}
	return sourceLabel(url) + ":" + id
}

var (
	setMu      sync.Mutex
	sets       = map[string]*source.Set{} // by URL
	ghClient   = source.NewClient()
	probsStore source.Store
)

// openSet returns the Set of url, loading its list on first use. On failure
// it shows why in c and returns nil.
func openSet(c *tgframe.Container, ctx context.Context, url string) *source.Set {
	setMu.Lock()
	defer setMu.Unlock()
	label := sourceLabel(url)
	s := sets[url]
	if s == nil {
		if probsStore == nil {
			probsStore = newProblemStore()
		}
		var err error
		s, err = source.New(url, ghClient, probsStore)
		if err != nil {
			tgcomp.MessageDanger(c, "題目來源 "+url+" 有誤："+err.Error())
			return nil
		}
		sets[url] = s
	}
	if s.Commit() != "" {
		return s
	}

	done := tgcomp.Spinner(c, "載入 "+label+"…")
	cached, err := s.Open(ctx)
	done()
	if err != nil {
		tgcomp.MessageDanger(c, "無法載入 "+label+"："+err.Error())
		tgcomp.Button(c, "重試", &tgcomp.ButtonConf{ID: "retry_" + url})
		return nil
	}
	if cached {
		// Show the stored list now; rerun the pages if a newer one comes.
		old := s.Commit()
		go func() {
			if err := s.Refresh(context.Background()); err != nil {
				log.Printf("檢查 %s 更新失敗：%v", label, err)
			} else if s.Commit() != old {
				app.RerunAll()
			}
		}()
	}
	return s
}

// loadedSet returns the Set of url if openSet made it.
func loadedSet(url string) *source.Set {
	setMu.Lock()
	defer setMu.Unlock()
	return sets[url]
}

// problemRef is where a problem of the catalog comes from.
type problemRef struct {
	set   *source.Set
	id    string // in set
	label string // of the source
}

// catalog is the problems of every source; entry IDs are problem keys.
type catalog struct {
	entries []source.Entry
	refs    map[string]problemRef
	labels  []string // of the loaded sources, in order
}

var (
	refsMu   sync.Mutex
	lastRefs = map[string]problemRef{} // of the last catalog, by key
)

// openCatalog loads every source; one that fails says why in c and is left out.
func openCatalog(c *tgframe.Container, ctx context.Context) *catalog {
	ct := &catalog{refs: map[string]problemRef{}}
	for _, url := range sourceURLs() {
		s := openSet(c, ctx, url)
		if s == nil {
			continue
		}
		label := sourceLabel(url)
		ct.labels = append(ct.labels, label)
		for _, e := range s.Entries() {
			key := problemKey(url, e.ID)
			if _, dup := ct.refs[key]; dup {
				continue // the same source twice, e.g. under two refs
			}
			ct.refs[key] = problemRef{set: s, id: e.ID, label: label}
			e.ID = key
			ct.entries = append(ct.entries, e)
		}
	}
	refsMu.Lock()
	lastRefs = ct.refs
	refsMu.Unlock()
	return ct
}

// problem loads the problem of key; its ID is the key.
func (ct *catalog) problem(ctx context.Context, key string) (*problems.Problem, error) {
	ref, ok := ct.refs[key]
	if !ok {
		return nil, fmt.Errorf("沒有題目 %s", key)
	}
	p, err := ref.set.Problem(ctx, ref.id)
	if err != nil {
		return nil, err
	}
	cp := *p // the Set caches p
	cp.ID = key
	return &cp, nil
}

// refOf returns where the problem of key comes from, per the last catalog.
func refOf(key string) (problemRef, bool) {
	refsMu.Lock()
	defer refsMu.Unlock()
	ref, ok := lastRefs[key]
	return ref, ok
}

// addSource appends url, saying in c why not if it can't be.
func addSource(c *tgframe.Container, url string) {
	if _, err := source.Parse(url); err != nil {
		tgcomp.MessageDanger(c, err.Error())
		return
	}
	urls := sourceURLs()
	if slices.Contains(urls, url) {
		tgcomp.MessageInfo(c, "已經有這個來源")
		return
	}
	setSourceURLs(append(urls, url))
}
