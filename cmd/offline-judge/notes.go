//go:build js && wasm

package main

import (
	"encoding/json"
	"slices"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"
)

// starred returns the keys of the starred problems.
func starred() []string {
	var ids []string
	_ = json.Unmarshal([]byte(memo.getText("starred", "[]")), &ids)
	return ids
}

// setStarred stars or unstars problem key.
func setStarred(key string, on bool) {
	ids := starred()
	has := slices.Contains(ids, key)
	switch {
	case on && !has:
		ids = append(ids, key)
	case !on && has:
		ids = slices.DeleteFunc(ids, func(id string) bool { return id == key })
	default:
		return
	}
	bs, _ := json.Marshal(ids)
	memo.setText("starred", string(bs))
}

// starBox draws the star toggle of pr in the sidebar.
func starBox(p *tgframe.Params, key string) {
	on := tgcomp.Checkbox(p.Sidebar, "★ 收藏這題", &tgcomp.CheckboxConf{
		Base:    tgframe.Base{ID: "star_" + key},
		Default: slices.Contains(starred(), key),
	})
	setStarred(key, on)
}

// noteSection draws the problem's notes, kept with the drafts.
func noteSection(c *tgframe.Container, key string) {
	note := memo.getText("note_"+key, "")
	box := tgcomp.Expand(c, "我的筆記", note != "", &tgcomp.ExpandConf{ID: "note_box_" + key})
	tgcomp.Caption(box, "解題想法、卡住的地方、要再看的題解；只存在這個瀏覽器，備份會帶上")
	note = tgcomp.Textarea(box, "筆記（Markdown）", &tgcomp.TextareaConf{
		ID:      "note_" + key,
		Height:  6,
		Default: note,
	})
	memo.setText("note_"+key, note)
	if note != "" {
		tgcomp.Markdown(box, note)
	}
}
