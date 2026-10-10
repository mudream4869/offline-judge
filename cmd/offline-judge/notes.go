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

// noteButton draws the sidebar button that opens the problem's notes in a
// dialog; notes are kept with the drafts.
func noteButton(p *tgframe.Params, key string) {
	label := "📝 我的筆記"
	if memo.getText("note_"+key, "") != "" {
		label += "（有內容）"
	}
	d := tgcomp.Dialog(p.Main, "我的筆記", &tgcomp.DialogConf{
		Base:  tgframe.Base{ID: "note_dialog_" + key},
		Width: tgcomp.DialogWidthLarge,
	})
	if tgcomp.Button(p.Sidebar, label, &tgcomp.ButtonConf{ID: "note_open_" + key}) {
		d.Open()
	}
	d.With(func(c *tgframe.Container) {
		tgcomp.Caption(c, "解題想法、卡住的地方、要再看的題解；只存在這個瀏覽器，備份會帶上")
		note := tgcomp.Textarea(c, "筆記（Markdown）", &tgcomp.TextareaConf{
			ID:      "note_" + key,
			Height:  10,
			Default: memo.getText("note_"+key, ""),
		})
		memo.setText("note_"+key, note)
		if note != "" {
			tgcomp.Markdown(c, note)
		}
	})
}
