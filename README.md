# Offline Judge

![Offline Judge](web/banner.webp)

完全在瀏覽器裡執行的 Python / C++ / JavaScript / Go 解題系統，不需要後端。

- UI、題目、評測邏輯：Go，用 [toolgui](https://github.com/voilelab/toolgui) 編成 wasm
- 執行 Python：[Pyodide](https://pyodide.org/)，跑在獨立的 module worker，超時直接 `terminate()`，並預先暖機一個備用 worker
- 執行 C++（PoC）：[YoWASP clang](https://yowasp.org/) 編成 WASI wasm，再用
  [browser_wasi_shim](https://github.com/bjorn3/browser_wasi_shim) 在可砍掉的 worker 裡執行
- 執行 JavaScript：直接用瀏覽器的 JS 引擎，在 worker 裡模擬 Node 的 stdin / stdout（見下方「JavaScript 的限制」）
- 執行 Go：Go 本身的 `cmd/compile`、`cmd/link` 編成 WASI wasm，在瀏覽器裡把程式編成 `GOOS=wasip1` 的 wasm，
  再跟 C++ 一樣執行（見下方「Go 的限制」）
- 測資不保密，題目從 GitHub 下載（見下方「題目來源」）
- 頁面：首頁、題目列表（`#/problems`，側欄可搜尋編號或題目、依標籤與狀態篩選，「狀態」欄標示有 AC 的「已通過」與提交過但沒 AC 的「未通過」，點表格裡的題目進入 `#/problems?id=…`，篩選條件也寫在網址裡，上一頁或「返回題目列表」會保留）、
  提交紀錄（`#/submissions`，所有題目的提交，點一筆看程式碼與結果，`#/submissions?id=…`）、設定（`#/settings`）
- 程式碼、自訂輸入、語言選擇與提交紀錄存在瀏覽器的 IndexedDB（`offline-judge`），重新整理後還在

## 架構

```
page ── toolgui worker (Go wasm：UI / 題目 / 比對)
            ├── pyworker.mjs   (Pyodide：收 code + stdin，回 stdout / stderr / 耗時)
            ├── cppcompile.mjs (clang：收 code，回 WebAssembly.Module 或編譯錯誤；常駐)
            ├── gocompile.mjs  (Go compile + link：收 code，回 WebAssembly.Module 或編譯錯誤；常駐)
            ├── wasirun.mjs    (C++、Go 共用：收 module + stdin，回 stdout / stderr / 耗時)
            ├── jsrun.mjs      (收 code + stdin，回 stdout / stderr / 耗時；每次執行換新的 worker)
            └── checker.mjs    (轉送給 data: URL worker 裡的 checkbox.mjs，跑題目的 checker.js，回 AC / WA 與訊息)

sandbox.mjs：跑題目附的 JS（checker.js、interactor.js）用，先拿掉儲存與網路 API
```

執行用的 worker 介面為：

```
in:  {id, code, stdin, interactor?, outputLimit?}   （wasirun.mjs 是 {id, module, stdin, interactor?, outputLimit?}）
out: {type: "ready"} | {type: "error", error}
     {type: "result", id, status: "ok"|"re"|"ole", stdout, stderr, ms, fatal, judged?, iaError?}
```

有 `interactor`（互動題）時 `stdin` 是互動程式的輸入，`stdout` 是互動過程，
`judged` 是互動程式的判定 `{ok, message}`。pyworker、wasirun 與 jsrun 都支援。

C++ 與 Go 的編譯與執行分開：載入編譯器很慢，所以編譯 worker 常駐；執行 worker 很便宜，
TLE 時直接砍掉。同一份程式碼只編譯一次，所有測資共用。

語言在第一次被選到時才開始載入。

## 判定

| 結果 | 條件 |
| --- | --- |
| AC | 輸出相符（預設忽略行尾空白與結尾空行，可用 `compare` 改變）；有 checker 的題目則由 checker 判定，互動題由互動程式判定 |
| WA | 輸出不符（會指出第一個不同的位置），或 checker / 互動程式不接受（互動題先於 RE：被互動程式切斷的程式常常接著出錯） |
| CE | 編譯錯誤（C++、Go），不執行任何測資 |
| RE | 例外、非零 `SystemExit` 或非零 exit code（含 `process.exit`） |
| TLE | 耗時超過限制；超過限制 +1 秒仍未結束就砍掉 worker |
| OLE | stdout 加 stderr 超過 16 MB（JavaScript 以字元數計），立即停止程式；不影響後面的測資 |
| SKIP | 第一筆 TLE 之後的測資不再執行（每次 TLE 都要重載 Pyodide）；有子任務時改為跳過所屬子任務都已失敗的測資 |

有子任務的題目另外計分：子任務的測資全部 AC 才拿到該子任務的分數，總分為各子任務分數相加。

尚未支援 MLE。

## 保存

| object store | 內容 |
| --- | --- |
| `drafts` | key 為 `code_<語言>_<題目>`、`stdin_<語言>_<題目>`、`lang`，值為字串；沒改過的不存，預設值改了會跟著變 |
| `submissions` | 每次提交一筆：題目、題目版本、語言、程式碼、時間、結果；只留第一筆失敗的輸出且截斷，以 `problem` 為索引 |
| `indexes` | key 為來源網址，值為該來源某個 commit 的檔案列表（JSON） |
| `blobs` | key 為 git blob sha，值為檔案內容；不同來源、不同 commit 的相同檔案共用 |

`drafts` 另外存 `sources`（題目來源）、`contests`（模擬賽）、`template_<語言>`（「設定」的預設程式碼，空字串為內建範本）、`starred`（收藏的題目，JSON 陣列）、`note_<題目>`（筆記）與顯示設定。

題目頁側欄可以「★ 收藏這題」，題目列表有 ★ 欄與「只看收藏」篩選；題目頁的「我的筆記」可以寫 Markdown 筆記，下方即時預覽。兩者都跟草稿存在一起，備份會帶上。

「設定 → 預設程式碼」可以改各語言的預設程式碼：沒改過的編輯器會跟著換，改過的保留，題目頁的「還原預設程式碼」會還原成它；函式題仍用題目附的 template。

「設定」的「備份」可以把 `drafts` 與 `submissions` 匯出成一個 JSON 檔（`indexes`、`blobs` 是題目快取，不匯出），
在另一個瀏覽器或清除資料後匯入。匯入只補上沒有的資料：已有的草稿與設定保留，題目來源與模擬賽合併，
提交紀錄依題目、語言、時間與程式碼去重；提交紀錄的編號沒被占用時沿用，順序不變。

「紀錄」分頁顯示該題最近 50 筆提交，每筆可展開刪除；「提交紀錄」頁面列出所有題目的全部提交。
寫入會等 IndexedDB transaction 提交才算保存成功。IndexedDB 無法使用或寫入失敗時，紀錄保留在記憶體，
標示「暫存」並提醒重新整理後會消失；恢復寫入後，新紀錄照常保存，既有暫存紀錄仍只留在這次開啟的頁面。
刪除暫存紀錄只移除記憶體裡的那筆；持久化紀錄刪除失敗時保留顯示，可再次嘗試。

## 題目來源

「設定」可以加入多個 GitHub 資料夾作為題目來源，題目列表合併顯示，側欄可依來源篩選。預設只有
`https://github.com/mudream4869/offline-judge/tree/main/problems`；「推薦來源」可以一鍵加入（清單在
`cmd/offline-judge/sources.go` 的 `recommended`）。
網址格式為 `https://github.com/<owner>/<repo>[/tree/<ref>[/<資料夾>]]`，ref 不能含 `/`，只支援公開 repo。

- 每個來源各自載入、檢查更新；某個來源載入失敗只在該處顯示錯誤，其他來源照常
- 題目在 app 裡的 id：預設來源的題目維持原本的 id（提交紀錄、草稿不受影響）；其他來源加上 `owner/repo/資料夾:`
  前綴（例如 `Kattis/problemtools/examples:hello`），避免不同來源撞名
- 以前只能設一個來源（`source`），會自動當成只有這一項的清單

- 載入列表：用 GitHub API 把 ref 解析成 commit，再一次列出整個 tree，並下載題目列表 `problems.json`（只有一個檔案）
- 開啟題目時才下載 `statement.md` 與測資，檔案來自 `raw.githubusercontent.com`（固定在該 commit），
  下載後用 blob sha 驗證並存進 IndexedDB
- 之後啟動先用快取顯示，背景只打 1 次 API 檢查 commit 有沒有變；有變才重新列 tree，沒變過的檔案不重抓
- 未登入的 GitHub API 每小時 60 次，正常使用每次啟動每個來源 1 次
- 開啟過的題目可離線使用；「設定」的「全部下載」可一次下載全部題目

抓取邏輯在 `internal/source`（用 Go 的 `net/http`，在瀏覽器裡走 fetch）：
`Backend` 負責列出與下載檔案（目前只有 GitHub），format 負責把檔案變成題目（本 repo 的格式與 Kattis）；
快取、離線與版本檢查在兩者之上共用。

### Kattis 題目包

來源資料夾沒有 `problems.json` 時，會當成 [Kattis 題目包](https://www.kattis.com/problem-package-format/)讀取：

- 資料夾本身有 `problem.yaml`：這個來源就是一題，id 是資料夾名稱（repo 根目錄則是 repo 名稱），
  例如 `https://github.com/Kattis/problemtools/tree/master/examples/hello`
- 子資料夾有 `problem.yaml`：每個子資料夾一題，資料夾名稱就是題目 id，
  例如 `https://github.com/Kattis/problemtools/tree/master/examples`

legacy 與 2023-07 之後的格式都可以：

| Kattis | 對應 |
| --- | --- |
| `name`（字串或各語言） | 題目名稱，優先順序 `zh-TW`、`zh`、`en`；沒有就用資料夾名稱 |
| `limits.time_limit` | 時間限制；legacy 格式沒有時限（原本由標準解推算），用 2 秒 |
| `keywords` | 標籤 |
| `statement/` 或 `problem_statement/` 的 `problem.<語言>.md`／`.tex` | 題目敘述；優先用 Markdown，`.tex` 會轉成 Markdown（章節、粗斜體、清單、簡單表格、數學式），圖片只顯示說明文字 |
| `data/sample/`、`data/secret/` 的 `.in`／`.ans` | 測資，名稱保留路徑（如 `secret/group1/01`） |
| 預設驗證器與 `validator_flags`／`output_validator_flags`／`output_validator_args` | `compare` 的 `tokens` 模式，參數相同 |
| 計分題的測資組（`data/secret/<組>/`） | 子任務：組內全對才得分（legacy 的 `grader_flags: min` + `accept_score`，或 2023-07 的 `max_score`）；`require_pass` 會把需要先過的組併進來。逐筆給分的組改成只判全對，並在敘述加註 |
| 自訂輸出驗證器、互動題、`include/`、submit-answer／multi-pass | 列表標「無法評測」，題目照樣可看 |

版本沒有標準欄位，用題目用到的檔案內容算出（例如 `3f2a9c1b0d4e`），測資或敘述改了就會把舊提交標成「舊版」。

## C++ 的限制（PoC）

- 編譯參數：`-std=c++17 -O2`，stack 64 MB
- `<bits/stdc++.h>` 是自己寫的（`web/stdc++.h`），只含常用標頭；在 build 時預先編成 PCH，
  有 include 它的程式才會用到
- `cin >>` 讀數字很慢，約為 `scanf` 的 1/40（libc++ 的解析本身慢，跟 I/O 無關）；輸入大的題目時限要算進去
- wasi 版 libc++ 不支援例外：`throw` 直接 abort 判 RE，`catch` 不會被執行
- 執行速度約為原生的 1/3（時間限制可依語言調整，見「新增題目」的 `time_limits_ms`）
- 第一次選 C++ 時才下載 clang（gzip 後約 27 MB）加上 PCH（約 10 MB）；編譯一次約 1 秒
- clang 與 PCH 是 lazy asset（`assets/cpp/`）：service worker 不預先快取，第一次用到時才存起來，
  之後 C++ 離線也能用

## JavaScript 的限制

- 只模擬解題常用的 Node API：
  - `require('fs')`：`readFileSync(0 | '/dev/stdin')`、`writeSync`
  - `require('readline')`：`createInterface` 的 `line` / `close` 事件與 `for await`
  - `process`：`stdin` 的 `data` / `end`、`stdout.write`、`exit`、`exitCode`、`hrtime`
  - `console.log` 的格式是 Node 的簡化版（陣列、物件只輸出一行）
  - 其他模組 `require` 會丟錯，判 RE
- 遞迴深度約 5000 層（Chromium 的 worker stack 較小，無法調整），太深會 `RangeError` 判 RE；
  深度大的 DFS 要改成迴圈
- 程式結束的判斷：stdin 事件送完、沒有未完成的 timer 就結束；卡在永遠不會 resolve 的 Promise 不算 TLE
- 互動題：`fs.readFileSync(0)` 每次回傳互動程式的下一段回答（互動程式結束時為空字串）；`readline` 與
  `process.stdin` 的事件在程式閒下來、等著輸入時才送下一段回答（範例：`problems/0009-guess-number/_solutions/ac.js`）

## Go 的限制

- 編譯參數：預設值（有最佳化），連結時 `-s -w`；第一次選 Go 時才下載 compile、link 與標準函式庫
  （gzip 後約 19 MB），編譯一次約 1 秒；執行速度約為原生的 1/2
- 只能 import 解題常用的標準函式庫：`bufio` `bytes` `cmp` `container/*` `errors` `fmt` `io` `maps`
  `math` `math/big` `math/bits` `math/rand` `math/rand/v2` `os` `regexp` `slices` `sort` `strconv`
  `strings` `time` `unicode` `unicode/utf8`（清單在 `scripts/build.sh` 的 `GO_PKGS`）；
  其他的會是 `could not import` 的 CE
- wasm 的呼叫堆疊受瀏覽器限制，遞迴深度約 2 萬層（Chromium），太深會 `RangeError` 判 RE
- 放在 `assets/go/`，跟 C++ 一樣第一次用到後可離線

## 統計

「統計」頁依這個瀏覽器裡的提交紀錄計算（`internal/stats`）：

- 已通過題數（只算目前題目來源裡的題目）、提交次數、AC 率、連續練習天數（到今天或昨天為止）
- 最近 30 天每天的提交數，分成 AC 與未通過
- 依標籤的完成度（開啟「顯示解法標籤」時也算解法標籤）、依語言的提交與 AC 率、各結果的比例

## 對拍

題目頁的「對拍」分頁：寫一個暴力解與一個測資產生器（跟目前的程式同一種語言），從「起始種子」開始跑「回合數」回合：

- 產生器從 stdin 讀入種子（一個整數），輸出一筆輸入；各語言有預設的產生器範本
- 暴力解與你的程式各跑一次，依題目的 `compare` 比對（有 `checker.js` 時以暴力解的輸出當參考答案）
- 停在第一個輸出不同或執行失敗（CE、RE、TLE、OLE）的回合，顯示輸入與並排的輸出
- 函式題的暴力解也只寫函式，由 grader 呼叫；互動題不能對拍
- 你的程式用題目的時限，產生器與暴力解至少 5 秒；結果不存成提交紀錄，程式碼跟草稿存在一起
- C++ 與 Go 會保留最近 4 次編譯的結果，三支程式輪流執行時不用重新編譯

## 模擬賽

「模擬賽」頁選幾題、設定時間（分鐘）後開始，計時期間的提交依 ICPC 規則計分：

- 計分板列出每題的解出時間（從開始算）與錯誤次數；有子任務的題目另列最高分
- 罰時是每題解出的時間（取整到分鐘）加上解出前每次錯誤 20 分鐘；CE 不算錯誤，沒解出的題目不算罰時
- 比賽中的題目不顯示提示與解法標籤；可以提前結束
- 比賽存在瀏覽器裡（跟草稿一起），結束後列在「過去的模擬賽」；計分從提交紀錄算出，刪掉提交紀錄會改變結果

## 新增題目

在 `problems/` 下開一個資料夾，再更新 `problems/problems.json`：

```
problems/0004-xxx/
  problem.json    {"title": "...", "time_limit_ms": 1000, "version": "2026-10-08 15:04:05", "tags": ["入門"],
                  "solution_tags": ["動態規劃"]}
  statement.md    題目敘述（Markdown，`$...$` / `$$...$$` 為 LaTeX 公式）；「## 提示」段落會收合顯示
  tests/
    sample1.in / sample1.out   sample 開頭的會顯示在題目裡
    01.in / 01.out
  checker.js      選填，答案不唯一時用（見下方）；浮點數誤差等只要設 problem.json 的 compare
  interactor.js   選填，互動題用（見下方）；與 checker.js 擇一
  grader/<語言>/   選填，函式題用（見下方）
  _solutions/     選填，參考解，給 scripts/bench.mjs 定時限用（見下方）；互動題必須有 ac.py
```

```sh
go test ./problems -update   # 從每題的 problem.json 重新產生 problems.json
```

`version` 是題目最後修改的時間（`YYYY-MM-DD hh:mm:ss`），改了敘述、測資或時間限制就要更新；
提交紀錄會記下評測時的版本，版本不同時標示「舊版」，並可在「紀錄」或提交紀錄頁按「用目前版本重新評測」，結果存成新的一筆。

`time_limits_ms` 選填，依語言（`py`、`cpp`、`js`、`go`）覆寫 `time_limit_ms`，沒列出的語言用 `time_limit_ms`。
只在其他語言用錯的複雜度也能過時才需要，例如 `0006-rmq` 的 `{"py": 6000, "js": 1000}`。
題目頁顯示目前語言的時限，題目列表顯示 `time_limit_ms` 與有覆寫的語言，例如 `4000 ms（Python 6000 ms、JavaScript 1000 ms）`。

`tags` 選填，會顯示在題目列表與題目頁，列表可依標籤篩選（選多個時只列出同時有這些標籤的題目）。

`tags` 只放不暴雷的題目性質（例如「入門」、「互動」、「多解」）；`solution_tags` 選填，放會暗示解法或領域的標籤（例如「取模」、「數論」、「資料結構」、「線段樹」），不要跟 `tags` 重複。
預設只在題目頁收合顯示；「設定」勾選「顯示解法標籤」後，才會跟 `tags` 一起顯示在列表與題目頁，也能用來篩選。
這個 repo 的題目 `tags` 與 `solution_tags` 至少要有一個（`go test` 會檢查）。

`subtasks` 選填，把測資分組計分（參考 TIOJ；範例：`problems/0006-rmq`）：

```json
"subtasks": [
  {"score": 40, "tests": ["01", "02", "03"], "constraints": "$N, Q \\le 1000$"},
  {"score": 60, "tests": ["0*"]}
]
```

- `tests` 是測資名稱或 [`path.Match`](https://pkg.go.dev/path#Match) 的樣式（例如 `1-*`），每個都要符合至少一筆測資；
  同一筆測資可以屬於多個子任務
- 除了 sample 以外，每筆測資都要屬於某個子任務；sample 沒列進子任務時照常執行，但不影響分數
- `constraints` 選填（Markdown），跟分數、測資一起列在題目頁的「子任務」表格
- 題目列表的「狀態」會顯示沒通過的題目的最高分；這個 repo 的題目分數總和要是 100（`go test` 會檢查）

`compare` 選填，內建的輸出比對方式（參考 TIOJ），不用寫 checker：

| `compare` | 比對方式 |
| --- | --- |
| `line`（預設） | 逐行比對，忽略行尾空白與結尾空行 |
| `strict` | 逐位元組比對 |
| `white-diff` | 逐行比對，每行以空白分隔成項，空白的數量不影響；忽略結尾空行 |
| `float-diff [absolute\|relative\|absolute-relative] [誤差]` | 同 `white-diff`，但答案中含 `.`、`e` 或 `E` 的數字允許誤差（預設 `absolute-relative 1e-6`，絕對或相對誤差其一在範圍內即可）；整數仍要相同 |
| `tokens [參數…]` | Kattis 預設驗證器：整份輸出以空白分隔逐項比對，不分換行、**不分大小寫**；參數同 Kattis：`case_sensitive`、`space_change_sensitive`、`float_tolerance ε`、`float_absolute_tolerance ε`、`float_relative_tolerance ε` |

WA 時會在「比對結果」指出第一個不同的行與項，並把預期輸出與你的輸出左右並排，
從第一個不同的行往上 5 行起列出 40 行，依 `compare` 判定逐行標出不同的行（`✗`），完整輸出收在下方。有 `checker.js` 或 `interactor.js` 時不能設定 `compare`。

`problems.json` 沒更新、或 `version` 格式不對的話 `go test` 會失敗。其他來源也要在資料夾根目錄放 `problems.json`：

```json
[{"id": "0001-a-plus-b", "title": "A + B", "time_limit_ms": 1000, "version": "2026-10-06 21:30:00"}]
```

推到來源的分支後，使用者下次開啟時就會拿到，不用重新 build。

題目頁底部有「回報題目問題」連結，開啟來源 repo 的新 issue，標題與內容預先填好題號、版本與來源 commit。

### 定時限

在 `_solutions/` 放各語言的參考解：`ac.<副檔名>` 是預期的解法，`tle.<副檔名>` 是複雜度錯、應該超時的解法
（副檔名 `py`、`cpp`、`js`、`go`）。資料夾以 `_` 開頭，go 工具才會忽略它，app 也不會下載。

```sh
scripts/build.sh                          # bench 用 dist/ 裡的 worker
node scripts/bench.mjs [題目...] [--runs N] [--cap 毫秒]
```

用 headless Chromium（Playwright）跑網站本身的 worker，列出每個解法最慢的測資耗時與時限的倍數。
啟動時會用 Go 編譯 `cmd/benchjudge` 到暫存目錄，每次執行的結果交給正式評測的 `judge.Judge` 判定，
共用 `compare` 模式與互動題的判定順序。checker 也使用網站的 worker，有 5 秒上限，
例外、逾時或無效回傳值會讓 benchmark 失敗。`--runs` 的每次執行都會判定，任何一次 WA / RE 都會讓 benchmark 失敗。
校準時允許程式執行到 `--cap`，量到的耗時再與題目時限比較，不會在題目時限到達時就中止。

```sh
node --test scripts/benchjudge.test.mjs    # 不需要 dist/ 或瀏覽器，驗證 benchmark 的判定規則
```

ac 超時或 tle 通過時，結束碼為 1；差距不到 2 倍時會提醒。時限建議至少是 ac 的 2 倍，
並且要明顯低於 tle（評測跑在使用者的機器上，可能比較慢）；兩者湊不出來時，應該加大測資，不要硬調時限。
`--cap` 是砍掉程式前的等待時間（預設 10000）；`--runs` 每筆測資跑多次，ac 取最慢、tle 取最快。

### checker

答案不唯一的題目放 `checker.js`，取代逐字比對（範例：`problems/0005-mode`）：

```js
// input：測資輸入，output：選手輸出，answer：.out 的參考答案
export default function check(input, output, answer) {
  return true            // AC
  // return false / '訊息'  WA，訊息會顯示在失敗的測資下
}
```

- 選手 TLE / RE 時不會呼叫 checker
- 在獨立的 worker 執行，每次 5 秒上限；丟例外、逾時或回傳其他型別都算評測失敗
- 跑在 `data:` URL 的 worker 裡，origin 是不透明的，瀏覽器不讓它碰這個網站的 IndexedDB、Cache Storage 與 OPFS；
  另外也拿掉 fetch、Worker 等 API。module 的 `import` 仍可連網，但 checker 只拿得到測資與選手輸出
- `go test ./problems` 會用 Node 確認每題的 `.out` 都能通過自己的 checker

### 互動題

放 `interactor.js` 就是互動題（範例：`problems/0009-guess-number`）。程式不讀固定的輸入，
而是跟互動程式一問一答；`.in` 是互動程式的輸入，`.out` 只有 sample 需要，放範例互動過程顯示在題目裡。

```js
// input：測資的 .in
export default function interact(input) {
  return {
    // 程式要讀但沒有資料時呼叫；out 是程式上次以來的輸出。回傳要給程式讀的字串，null 為 EOF
    read(out) { return '...\n' },
    // 程式結束時呼叫；out 是剩下的輸出。回傳 true 為 AC，false / '訊息' 為 WA
    finish(out) { return true },
  }
}
```

- 互動程式跟選手程式在同一個 worker 裡同步呼叫（不需要 `SharedArrayBuffer`），
  所以 TLE 時一起被砍；花在互動程式的時間不算進選手的耗時
- `read` 只在程式要讀時才呼叫，程式一次輸出多行時會一起給；回傳 null 後程式讀到 EOF，不會兩邊互等
- 互動過程以 `→`（程式輸出）、`←`（互動程式回答）記錄，失敗時顯示在測資下
- Python 與 C++ 的輸出不 flush 也送得到（C++ 的 stdout 是 line buffered）；Go 直接寫 `os.Stdout` 也是，
  但用 `bufio.Writer`（範本預設）時要在讀之前 `Flush()`。JavaScript 的 `console.log` / `process.stdout.write`
  也直接送到；`fs.readFileSync(0)` 每次回傳互動程式的下一段回答，`readline` 與 `process.stdin` 的事件則在程式閒下來
  （沒有待執行的事件與 timer）、等著輸入時送來下一段回答
- 跟 checker 一樣先拿掉儲存與網路 API，但跟選手程式在同一個 worker、與網站同 origin，只能算盡力而為；丟例外算評測失敗
- `go test ./problems` 會用 Node 讓 `_solutions/ac.py` 跟互動程式對答，所有測資都要通過；
  這裡互動程式在程式輸出完整的一行後就被呼叫，參考解要 `flush=True`

### 函式題

選手只寫函式，輸入輸出由題目的 grader 處理（參考 TIOJ 的 interlib；範例：`problems/0011-max-subarray`）：

```
grader/py/grader.py      主程式，用 from solution import ... 呼叫選手的函式
grader/py/template.py    選填，編輯器的預設程式碼（函式空殼）；按編輯器下方「還原預設程式碼」可回到它
grader/cpp/grader.cpp    主程式，宣告函式原型；與選手的 solution.cpp 一起編譯連結
grader/js/grader.js      主程式，用 require('./solution') 取得選手 module.exports 的函式
grader/go/grader.go      主程式（package main）；與選手的 solution.go 一起編譯
```

- 有任何一個 grader 就是函式題；沒有 grader 的語言不能提交。一種語言一個資料夾，Go 與 C++ 檔案才不會混在同一個 package
- 測資的 `.in` / `.out` 是 grader 的輸入輸出，比對方式照常（`compare`、`checker.js`）；可以跟 `interactor.js` 並用
- 選手的程式碼在錯誤訊息裡叫 `solution.py`、`solution.cpp`、`solution.js`、`solution.go`
- `go test ./problems` 會用本機的 python3 跑 `_solutions/ac.py` 與 `grader.py`，所有測資都要通過；
  函式題要有全部 4 種語言的 grader。`scripts/bench.mjs` 也會帶上 grader

## 開發

需要 Go 1.27.2+ 與 Node（用 `npm pack` 抓 Pyodide、clang、WASI shim，並用 Node 跑 clang 產生 PCH）。

```sh
go test ./...
node scripts/test-browser.mjs   # Playwright Chromium：真正 IndexedDB 的 js/wasm Go 測試
scripts/build.sh          # 輸出到 dist/
scripts/build.sh serve    # http://localhost:3000
```

`build.sh` 是包一層 toolgui 的 `go tool toolgui-wasm build|serve`，多做的事是把
Pyodide、WASI shim 與 `web/` 準備到 `.cache/assets`，並把 clang 與 PCH、
`GOOS=wasip1` 的 Go compile、link 與標準函式庫（`std.tar`）準備到 `.cache/lazy`（`-lazy-assets`）。跑過一次 `build.sh` 後，也可以直接用：

```sh
go tool toolgui-wasm serve -o dist -assets .cache/assets -lazy-assets .cache/lazy ./cmd/offline-judge
```

直接 serve 時不帶 `-assets` 的話，頁面能開，但沒有 Python / C++ / JavaScript / Go 環境可以執行；
不帶 `-lazy-assets` 的話 C++、Go 不能用。

`dist/` 是靜態網站，可直接放到 GitHub Pages（見 `.github/workflows/pages.yml`，
需在 repo 設定把 Pages 來源設為 GitHub Actions）。
需要 https 或 localhost（toolgui 的 OPFS 需要 secure context）。

可安裝成 web app（PWA）：manifest 在 `pwa/manifest.json`，圖示在 `pwa/icons/`（build 時複製到 `assets/icons/`）；
iOS 的 `apple-touch-icon` 等 head 標籤由 toolgui 依 manifest 產生。
