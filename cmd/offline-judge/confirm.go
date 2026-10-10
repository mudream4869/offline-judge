//go:build js && wasm

package main

import (
	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgcomp/tcutil"
	"github.com/voilelab/toolgui/toolgui/tgframe"
)

// confirmDialog asks before an action that can't be undone. One per page:
// the pending action is kept in state, so every row shares it.
type confirmDialog struct {
	d     *tgcomp.DialogContainer
	owner *tgframe.Container
	id    string
}

// confirmAsk is what is pending: the action, the question, the OK label.
type confirmAsk struct {
	Action, Msg, OK string
}

// newConfirm creates page's dialog; call it before any click handling.
func newConfirm(p *tgframe.Params, page string) *confirmDialog {
	id := "confirm_" + page
	return &confirmDialog{
		d:     tgcomp.Dialog(p.Main, "確認", &tgcomp.DialogConf{Base: tgframe.Base{ID: id}}),
		owner: p.Main,
		id:    id,
	}
}

func (cf *confirmDialog) pending() confirmAsk {
	a, _ := cf.owner.State.Get[confirmAsk](cf.id + "_ask")
	return a
}

func (cf *confirmDialog) okConf() *tgcomp.ButtonConf {
	return &tgcomp.ButtonConf{ID: cf.id + "_ok", Color: tcutil.ColorDanger}
}

func (cf *confirmDialog) cancelConf() *tgcomp.ButtonConf {
	return &tgcomp.ButtonConf{ID: cf.id + "_cancel"}
}

// confirmed returns the action confirmed this run, or "".
func (cf *confirmDialog) confirmed() string {
	a := cf.pending()
	if tgcomp.ButtonClicked(cf.owner, "取消", cf.cancelConf()) {
		cf.d.Close()
		return ""
	}
	if !tgcomp.ButtonClicked(cf.owner, a.OK, cf.okConf()) {
		return ""
	}
	cf.d.Close()
	return a.Action
}

// ask opens the dialog for action; ok labels the confirm button.
func (cf *confirmDialog) ask(action, msg, ok string) {
	cf.owner.State.Set(cf.id+"_ask", confirmAsk{Action: action, Msg: msg, OK: ok})
	cf.d.Open()
}

// draw writes the dialog's body; call it after every ask.
func (cf *confirmDialog) draw() {
	cf.d.With(func(c *tgframe.Container) {
		a := cf.pending()
		tgcomp.Text(c, a.Msg)
		ok, cancel := tgcomp.EqColumn2(c, &tgcomp.ColumnConf{ID: cf.id + "_buttons"})
		tgcomp.Button(ok, a.OK, cf.okConf())
		tgcomp.Button(cancel, "取消", cf.cancelConf())
	})
}
