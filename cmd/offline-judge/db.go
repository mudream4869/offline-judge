//go:build js && wasm

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"syscall/js"
	"time"

	"github.com/mudream4869/offline-judge/internal/source"
)

// IndexedDB keeps drafts and submissions across reloads.
// Every call here blocks: never make one from a js.FuncOf callback.

const (
	dbName     = "offline-judge"
	dbVersion  = 2
	draftStore = "drafts"      // key → text
	subStore   = "submissions" // {id, problem, version, lang, code, at, report}
	indexStore = "indexes"     // source URL → source.Index as JSON
	blobStore  = "blobs"       // git blob sha → Uint8Array
)

var (
	dbOnce  sync.Once
	dbVal   js.Value // undefined when IndexedDB can't be used
	errNoDB = errors.New("no IndexedDB")
)

func db() js.Value {
	dbOnce.Do(func() {
		v, err := openDB()
		if err != nil {
			log.Printf("IndexedDB 無法使用，程式碼與提交紀錄不會保存：%v", err)
			return
		}
		dbVal = v
	})
	return dbVal
}

func openDB() (js.Value, error) {
	idb := js.Global().Get("indexedDB")
	if !idb.Truthy() {
		return js.Value{}, errNoDB
	}
	req, err := jsTry(func() js.Value { return idb.Call("open", dbName, dbVersion) })
	if err != nil {
		return js.Value{}, err
	}
	upgrade := js.FuncOf(func(_ js.Value, args []js.Value) any {
		d := req.Get("result")
		if args[0].Get("oldVersion").Int() < 1 {
			d.Call("createObjectStore", draftStore)
			s := d.Call("createObjectStore", subStore,
				map[string]any{"keyPath": "id", "autoIncrement": true})
			s.Call("createIndex", "problem", "problem")
		}
		d.Call("createObjectStore", indexStore)
		d.Call("createObjectStore", blobStore)
		return nil
	})
	defer upgrade.Release()
	req.Set("onupgradeneeded", upgrade)
	return await(req)
}

// await waits for an IDBRequest and returns its result.
func await(req js.Value) (js.Value, error) {
	ch := make(chan error, 1)
	ok := js.FuncOf(func(js.Value, []js.Value) any {
		ch <- nil
		return nil
	})
	fail := js.FuncOf(func(js.Value, []js.Value) any {
		ch <- jsErr(req.Get("error"))
		return nil
	})
	defer ok.Release()
	defer fail.Release()
	req.Set("onsuccess", ok)
	req.Set("onerror", fail)
	if err := <-ch; err != nil {
		return js.Value{}, err
	}
	return req.Get("result"), nil
}

// jsTry runs f, returning what JavaScript throws as an error.
func jsTry(f func() js.Value) (v js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(js.Error)
			if !ok {
				panic(r)
			}
			err = jsErr(e.Value)
		}
	}()
	return f(), nil
}

func jsErr(v js.Value) error {
	if v.Type() != js.TypeObject {
		return errors.New("unknown IndexedDB error")
	}
	return fmt.Errorf("%s: %s", v.Get("name").String(), v.Get("message").String())
}

// objStore opens a one-store transaction.
func objStore(name, mode string) (js.Value, error) {
	d := db()
	if d.IsUndefined() {
		return js.Value{}, errNoDB
	}
	return jsTry(func() js.Value {
		return d.Call("transaction", name, mode).Call("objectStore", name)
	})
}

// request makes one request on a store and waits for it.
func request(name, mode string, f func(s js.Value) js.Value) (js.Value, error) {
	s, err := objStore(name, mode)
	if err != nil {
		return js.Value{}, err
	}
	var tx *transactionWaiter
	if mode == "readwrite" {
		tx = watchTransaction(s.Get("transaction"))
		defer tx.close()
	}
	req, err := jsTry(func() js.Value { return f(s) })
	if err != nil {
		if tx != nil {
			_, _ = jsTry(func() js.Value { return tx.tx.Call("abort") })
		}
		return js.Value{}, err
	}
	if tx != nil {
		if err := <-tx.done; err != nil {
			return js.Value{}, err
		}
		if v := req.Get("error"); v.Truthy() {
			return js.Value{}, jsErr(v)
		}
		return req.Get("result"), nil
	}
	return await(req)
}

// A successful request can still be rolled back. Writes wait for commit.
type transactionWaiter struct {
	tx              js.Value
	done            chan error
	complete, abort js.Func
}

func watchTransaction(tx js.Value) *transactionWaiter {
	w := &transactionWaiter{tx: tx, done: make(chan error, 1)}
	w.complete = js.FuncOf(func(js.Value, []js.Value) any {
		w.done <- nil
		return nil
	})
	w.abort = js.FuncOf(func(js.Value, []js.Value) any {
		err := errors.New("IndexedDB transaction aborted")
		if v := tx.Get("error"); v.Truthy() {
			err = jsErr(v)
		}
		w.done <- err
		return nil
	})
	tx.Set("oncomplete", w.complete)
	tx.Set("onabort", w.abort)
	return w
}

func (w *transactionWaiter) close() {
	w.tx.Set("oncomplete", js.Null())
	w.tx.Set("onabort", js.Null())
	w.complete.Release()
	w.abort.Release()
}

// logDBErr logs err, except errNoDB which db() already logged.
func logDBErr(what string, err error) {
	if err != nil && err != errNoDB {
		log.Printf("IndexedDB %s：%v", what, err)
	}
}

func loadDraft(key string) (string, bool) {
	v, err := request(draftStore, "readonly", func(s js.Value) js.Value {
		return s.Call("get", key)
	})
	logDBErr("讀取", err)
	if err != nil || v.Type() != js.TypeString {
		return "", false
	}
	return v.String(), true
}

func saveDraft(key, text string) {
	_, err := request(draftStore, "readwrite", func(s js.Value) js.Value {
		return s.Call("put", text, key)
	})
	logDBErr("寫入", err)
}

// saveSubmission stores s and sets its ID.
func saveSubmission(s *submission) error {
	rep, err := json.Marshal(s.Report)
	if err != nil {
		return err
	}
	v, err := request(subStore, "readwrite", func(st js.Value) js.Value {
		return st.Call("add", map[string]any{
			"problem": s.Problem,
			"version": s.Version,
			"lang":    s.Lang,
			"code":    s.Code,
			"at":      float64(s.At.UnixMilli()),
			"report":  string(rep),
		})
	})
	if err != nil {
		return err
	}
	s.ID = v.Int()
	return nil
}

func deleteSubmission(id int) error {
	_, err := request(subStore, "readwrite", func(s js.Value) js.Value {
		return s.Call("delete", id)
	})
	return err
}

// loadSubmissions returns up to n submissions of a problem, newest first.
func loadSubmissions(problem string, n int) ([]*submission, error) {
	return scanSubmissions(n, func(st js.Value) js.Value {
		only := js.Global().Get("IDBKeyRange").Call("only", problem)
		return st.Call("index", "problem").Call("openCursor", only, "prev")
	})
}

// loadAllSubmissions returns every submission, newest first.
func loadAllSubmissions() ([]*submission, error) {
	return scanSubmissions(math.MaxInt, func(st js.Value) js.Value {
		return st.Call("openCursor", nil, "prev")
	})
}

// scanSubmissions reads up to n rows from the cursor that open makes.
func scanSubmissions(n int, open func(st js.Value) js.Value) ([]*submission, error) {
	st, err := objStore(subStore, "readonly")
	if err != nil {
		return nil, err
	}
	req, err := jsTry(func() js.Value { return open(st) })
	if err != nil {
		return nil, err
	}

	var out []*submission
	ch := make(chan error, 1)
	// onsuccess fires once per row; stop by not calling continue.
	next := js.FuncOf(func(js.Value, []js.Value) any {
		cur := req.Get("result")
		if cur.IsNull() || len(out) >= n {
			ch <- nil
			return nil
		}
		if s, err := decodeSubmission(cur.Get("value")); err == nil {
			out = append(out, s)
		} else {
			log.Printf("略過損壞的提交紀錄：%v", err)
		}
		cur.Call("continue")
		return nil
	})
	fail := js.FuncOf(func(js.Value, []js.Value) any {
		ch <- jsErr(req.Get("error"))
		return nil
	})
	defer next.Release()
	defer fail.Release()
	req.Set("onsuccess", next)
	req.Set("onerror", fail)
	if err := <-ch; err != nil {
		return nil, err
	}
	return out, nil
}

func decodeSubmission(v js.Value) (*submission, error) {
	s := &submission{
		ID:      v.Get("id").Int(),
		Problem: v.Get("problem").String(),
		Lang:    v.Get("lang").String(),
		Code:    v.Get("code").String(),
		At:      time.UnixMilli(int64(v.Get("at").Float())),
	}
	if ver := v.Get("version"); ver.Type() == js.TypeString {
		s.Version = ver.String()
	}
	if err := json.Unmarshal([]byte(v.Get("report").String()), &s.Report); err != nil {
		return nil, err
	}
	return s, nil
}

// idbStore is a source.Store in IndexedDB. What can't be written (e.g. over
// quota) is kept in memory instead.
type idbStore struct {
	mem *source.MemStore
}

// newProblemStore returns a source.Store, in memory without IndexedDB.
func newProblemStore() source.Store {
	if db().IsUndefined() {
		return source.NewMemStore()
	}
	return idbStore{mem: source.NewMemStore()}
}

func (st idbStore) Index(url string) (*source.Index, bool) {
	if ix, ok := st.mem.Index(url); ok {
		return ix, true
	}
	v, err := request(indexStore, "readonly", func(s js.Value) js.Value {
		return s.Call("get", url)
	})
	logDBErr("讀取", err)
	if err != nil || v.Type() != js.TypeString {
		return nil, false
	}
	var ix source.Index
	if err := json.Unmarshal([]byte(v.String()), &ix); err != nil {
		log.Printf("略過損壞的題目列表快取：%v", err)
		return nil, false
	}
	return &ix, true
}

func (st idbStore) SetIndex(url string, ix *source.Index) {
	bs, err := json.Marshal(ix)
	if err == nil {
		_, err = request(indexStore, "readwrite", func(s js.Value) js.Value {
			return s.Call("put", string(bs), url)
		})
	}
	if err != nil {
		logDBErr("寫入", err)
		st.mem.SetIndex(url, ix)
	}
}

func (st idbStore) Blob(sha string) ([]byte, bool) {
	if bs, ok := st.mem.Blob(sha); ok {
		return bs, true
	}
	v, err := request(blobStore, "readonly", func(s js.Value) js.Value {
		return s.Call("get", sha)
	})
	logDBErr("讀取", err)
	if err != nil || !v.InstanceOf(js.Global().Get("Uint8Array")) {
		return nil, false
	}
	bs := make([]byte, v.Length())
	js.CopyBytesToGo(bs, v)
	return bs, true
}

func (st idbStore) SetBlob(sha string, data []byte) {
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	_, err := request(blobStore, "readwrite", func(s js.Value) js.Value {
		return s.Call("put", arr, sha)
	})
	if err != nil {
		logDBErr("寫入", err)
		st.mem.SetBlob(sha, data)
	}
}

func (st idbStore) BlobSHAs() []string {
	out := st.mem.BlobSHAs()
	v, err := request(blobStore, "readonly", func(s js.Value) js.Value {
		return s.Call("getAllKeys")
	})
	logDBErr("讀取", err)
	if err != nil {
		return out
	}
	for i := range v.Length() {
		out = append(out, v.Index(i).String())
	}
	return out
}
