//go:build js && wasm

package main

import (
	"fmt"
	"io"
	"time"

	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"

	"github.com/mudream4869/offline-judge/internal/backup"
)

// backupSection draws 設定's export and import.
func backupSection(p *tgframe.Params) {
	tgcomp.Subtitle(p.Main, "備份")
	tgcomp.Caption(p.Main, "程式碼、提交紀錄、模擬賽與設定只存在這個瀏覽器裡；清除網站資料就會不見。"+
		"匯出成檔案保存，或在另一台電腦、另一個瀏覽器匯入")
	tgcomp.DownloadFileFunc(p.Main, "匯出", exportBackup, &tgcomp.DownloadFileConf{
		Base:     tgframe.Base{ID: "backup_export"},
		MIME:     "application/json",
		Filename: "offline-judge-" + time.Now().Format("20060102") + ".json",
	})

	f := tgcomp.FileUpload(p.Main, "匯入備份檔", ".json", &tgcomp.FileUploadConf{
		Base: tgframe.Base{ID: "backup_file"},
	})
	tgcomp.Caption(p.Main, "匯入只會補上這裡沒有的資料，不會覆蓋：已有的程式碼與設定保留，"+
		"提交紀錄與模擬賽合併，重複的略過")
	if !tgcomp.Button(p.Main, "匯入", &tgcomp.ButtonConf{ID: "backup_import", Disabled: f == nil}) || f == nil {
		return
	}
	drafts, subs, err := importBackup(f)
	if err != nil {
		tgcomp.MessageDanger(p.Main, "匯入失敗："+err.Error())
		return
	}
	tgcomp.MessageSuccess(p.Main, fmt.Sprintf("已匯入 %d 筆提交紀錄、%d 項程式碼與設定", subs, drafts))
}

func exportBackup() ([]byte, error) {
	drafts, err := loadAllDrafts()
	if err != nil {
		return nil, fmt.Errorf("讀取程式碼與設定失敗：%w", err)
	}
	return backup.Encode(backup.Data{Drafts: drafts, Submissions: memo.allSubmissions()}, time.Now())
}

// importBackup merges the backup file f in; it returns how many drafts and
// submissions it added.
func importBackup(f *tgcomp.FileObject) (int, int, error) {
	r, err := f.Open()
	if err != nil {
		return 0, 0, err
	}
	defer r.Close()
	bs, err := io.ReadAll(r)
	if err != nil {
		return 0, 0, err
	}
	in, err := backup.Decode(bs)
	if err != nil {
		return 0, 0, err
	}
	local, err := loadAllDrafts()
	if err != nil {
		return 0, 0, fmt.Errorf("讀取程式碼與設定失敗：%w", err)
	}
	have := memo.allSubmissions()
	drafts, subs := backup.Merge(backup.Data{Drafts: local, Submissions: have}, in)

	for k, v := range drafts {
		memo.setText(k, v)
	}
	taken := map[int]bool{}
	for _, s := range have {
		taken[s.ID] = true
	}
	added := 0
	for _, s := range subs {
		// Keep the ID if free, so the order stays as it was.
		id := s.ID
		if id <= 0 || taken[id] {
			id = 0
		}
		if err := putSubmission(s, id); err != nil {
			memo.history.Forget()
			return len(drafts), added, fmt.Errorf("寫入提交紀錄失敗（已匯入 %d 筆）：%w", added, err)
		}
		taken[s.ID] = true
		added++
	}
	memo.history.Forget()
	return len(drafts), added, nil
}
