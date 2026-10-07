//go:build js && wasm

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"syscall/js"
	"time"

	"github.com/mudream4869/offline-judge/internal/judge"
)

// IndexedDB keeps drafts and submissions across reloads.
// Every call here blocks: never make one from a js.FuncOf callback.

const (
	dbName     = "offline-judge"
	dbVersion  = 1
	draftStore = "drafts"      // key → text
	subStore   = "submissions" // {id, problem, lang, code, at, report}
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
	upgrade := js.FuncOf(func(js.Value, []js.Value) any {
		d := req.Get("result")
		d.Call("createObjectStore", draftStore)
		s := d.Call("createObjectStore", subStore,
			map[string]any{"keyPath": "id", "autoIncrement": true})
		s.Call("createIndex", "problem", "problem")
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
	req, err := jsTry(func() js.Value { return f(s) })
	if err != nil {
		return js.Value{}, err
	}
	return await(req)
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

// submission is one judged submission.
type submission struct {
	ID      int
	Problem string
	Lang    string // lang.id
	Code    string
	At      time.Time
	Report  judge.Report
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

func deleteSubmission(id int) {
	_, err := request(subStore, "readwrite", func(s js.Value) js.Value {
		return s.Call("delete", id)
	})
	logDBErr("刪除", err)
}

// loadSubmissions returns up to n submissions of a problem, newest first.
func loadSubmissions(problem string, n int) ([]*submission, error) {
	st, err := objStore(subStore, "readonly")
	if err != nil {
		return nil, err
	}
	req, err := jsTry(func() js.Value {
		only := js.Global().Get("IDBKeyRange").Call("only", problem)
		return st.Call("index", "problem").Call("openCursor", only, "prev")
	})
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
	if err := json.Unmarshal([]byte(v.Get("report").String()), &s.Report); err != nil {
		return nil, err
	}
	return s, nil
}
