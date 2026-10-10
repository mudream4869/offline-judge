//go:build js && wasm

// Command offline-judge is a Python / C++ / JavaScript / Go judge that runs entirely in the browser.
package main

import (
	"github.com/voilelab/toolgui/toolgui/tgcomp"
	"github.com/voilelab/toolgui/toolgui/tgframe"
	"github.com/voilelab/toolgui/toolgui/tgwasm"
)

const intro = `完全在瀏覽器裡執行的 Python / C++ / JavaScript / Go 解題系統，不需要後端。

- 到「題目列表」點一題，寫好程式後「提交」即可評測
- 「測試範例」只跑範例、不存紀錄；「自訂輸入」可以用自己的輸入先跑跑看
- 「模擬賽」選幾題、限時作答，結束後看解題時間與罰時
- 「統計」看解題數、最近 30 天的提交、各標籤與語言的進度
- 測資不保密，失敗時會顯示第一筆錯誤的完整輸入與輸出
- 題目從 GitHub 下載，開啟過的題目可離線使用；來源可在「設定」更改`

const about = `**Offline Judge**：完全在瀏覽器裡執行的解題系統，不需要後端。

原始碼：[mudream4869/offline-judge](https://github.com/mudream4869/offline-judge)`

// Index is the home page.
func Index(p *tgframe.Params) error {
	tgcomp.Image(p.Main, "assets/banner.webp", &tgcomp.ImageConf{Width: "100%"})
	tgcomp.Markdown(p.Main, intro)
	tgcomp.PageLink(p.Main, "前往題目列表", "problems", nil)
	return nil
}

// app reruns open pages when background loads finish.
var app = tgframe.NewApp()

func main() {
	// Python is the default language: start loading it now.
	langs[0].runner()

	app.SetTitle("Offline Judge")
	app.SetAbout(about)
	app.AddPageByConfig(&tgframe.PageConfig{Name: "index", Title: "首頁", Emoji: "🏠"}, Index)
	app.AddPageByConfig(&tgframe.PageConfig{Name: "problems", Title: "題目列表", Emoji: "📚"}, Problems)
	app.AddPageByConfig(&tgframe.PageConfig{Name: "contest", Title: "模擬賽", Emoji: "🏁"}, Contests)
	app.AddPageByConfig(&tgframe.PageConfig{Name: "submissions", Title: "提交紀錄", Emoji: "📝"}, Submissions)
	app.AddPageByConfig(&tgframe.PageConfig{Name: "stats", Title: "統計", Emoji: "📊"}, Stats)
	app.AddPageByConfig(&tgframe.PageConfig{Name: "settings", Title: "設定", Emoji: "⚙️"}, Settings)
	app.SetHashPageNameMode(true)
	tgwasm.NewExecutor(app).Run()
}
