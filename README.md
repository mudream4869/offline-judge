# Offline Judge

完全在瀏覽器裡執行的 Python / C++ / JavaScript 解題系統，不需要後端。

- UI、題目、評測邏輯：Go，用 [toolgui](https://github.com/voilelab/toolgui) 編成 wasm
- 執行 Python：[Pyodide](https://pyodide.org/)，跑在獨立的 module worker，超時直接 `terminate()`，並預先暖機一個備用 worker
- 執行 C++（PoC）：[YoWASP clang](https://yowasp.org/) 編成 WASI wasm，再用
  [browser_wasi_shim](https://github.com/bjorn3/browser_wasi_shim) 在可砍掉的 worker 裡執行
- 執行 JavaScript：直接用瀏覽器的 JS 引擎，在 worker 裡模擬 Node 的 stdin / stdout（見下方「JavaScript 的限制」）
- 測資不保密，題目從 GitHub 下載（見下方「題目來源」）
- 頁面：首頁、題目列表（`#/problems`，點表格裡的題目進入，「返回題目列表」回到列表）、
  提交紀錄（`#/submissions`，所有題目的提交，點一筆看程式碼與結果）、設定（`#/settings`）
- 程式碼、自訂輸入、語言選擇與提交紀錄存在瀏覽器的 IndexedDB（`offline-judge`），重新整理後還在

## 架構

```
page ── toolgui worker (Go wasm：UI / 題目 / 比對)
            ├── pyworker.mjs   (Pyodide：收 code + stdin，回 stdout / stderr / 耗時)
            ├── cppcompile.mjs (clang：收 code，回 WebAssembly.Module 或編譯錯誤；常駐)
            ├── cpprun.mjs     (收 module + stdin，回 stdout / stderr / 耗時)
            └── jsrun.mjs      (收 code + stdin，回 stdout / stderr / 耗時；每次執行換新的 worker)
```

執行用的 worker 介面為：

```
in:  {id, code, stdin}          （cpprun.mjs 是 {id, module, stdin}）
out: {type: "ready"} | {type: "error", error}
     {type: "result", id, status: "ok"|"re", stdout, stderr, ms, fatal}
```

C++ 的編譯與執行分開：載入 clang 很慢，所以編譯 worker 常駐；執行 worker 很便宜，
TLE 時直接砍掉。同一份程式碼只編譯一次，所有測資共用。

語言在第一次被選到時才開始載入。

## 判定

| 結果 | 條件 |
| --- | --- |
| AC | 輸出相符（忽略行尾空白與結尾空行） |
| WA | 輸出不符 |
| CE | 編譯錯誤（C++），不執行任何測資 |
| RE | 例外、非零 `SystemExit` 或非零 exit code（含 `process.exit`） |
| TLE | 耗時超過限制；超過限制 +1 秒仍未結束就砍掉 worker |
| SKIP | 第一筆 TLE 之後的測資不再執行（每次 TLE 都要重載 Pyodide） |

尚未支援 MLE。

## 保存

| object store | 內容 |
| --- | --- |
| `drafts` | key 為 `code_<語言>_<題目>`、`stdin_<語言>_<題目>`、`lang`，值為字串；沒改過的不存，預設值改了會跟著變 |
| `submissions` | 每次提交一筆：題目、題目版本、語言、程式碼、時間、結果；只留第一筆失敗的輸出且截斷，以 `problem` 為索引 |
| `indexes` | key 為來源網址，值為該來源某個 commit 的檔案列表（JSON） |
| `blobs` | key 為 git blob sha，值為檔案內容；不同來源、不同 commit 的相同檔案共用 |

`drafts` 另外存 `source`（題目來源）。

「紀錄」分頁顯示該題最近 50 筆提交，每筆可展開刪除；「提交紀錄」頁面列出所有題目的全部提交。IndexedDB 無法使用（例如被瀏覽器封鎖）時照常運作，只是不會保存。

## 題目來源

在「設定」填 GitHub 上的資料夾網址，預設為
`https://github.com/mudream4869/offline-judge/tree/main/problems`。
格式為 `https://github.com/<owner>/<repo>[/tree/<ref>[/<資料夾>]]`，ref 不能含 `/`，只支援公開 repo。

- 載入列表：用 GitHub API 把 ref 解析成 commit，再一次列出整個 tree，並下載題目列表 `problems.json`（只有一個檔案）
- 打開題目時才下載 `statement.md` 與測資，檔案來自 `raw.githubusercontent.com`（固定在該 commit），
  下載後用 blob sha 驗證並存進 IndexedDB
- 之後啟動先用快取顯示，背景只打 1 次 API 檢查 commit 有沒有變；有變才重新列 tree，沒變過的檔案不重抓
- 未登入的 GitHub API 每小時 60 次，正常使用每次啟動 1 次
- 打開過的題目可離線使用；「設定」的「全部下載」可一次下載全部題目

抓取邏輯在 `internal/source`（用 Go 的 `net/http`，在瀏覽器裡走 fetch）。

## C++ 的限制（PoC）

- 編譯參數：`-std=c++17 -O2`，stack 64 MB
- `<bits/stdc++.h>` 是自己寫的（`web/stdc++.h`），只含常用標頭；在 build 時預先編成 PCH，
  有 include 它的程式才會用到
- wasi 版 libc++ 不支援例外：`throw` 直接 abort 判 RE，`catch` 不會被執行
- 執行速度約為原生的 1/3，時間限制目前沒有依語言調整
- 第一次選 C++ 時才下載 clang（gzip 後約 27 MB）加上 PCH（約 10 MB）；編譯一次約 1 秒
- clang 與 PCH 放在 `dist/cpp/`，不在 `assets/` 裡，所以 `-offline` 的 service worker
  不會預先快取它們：C++ 離線時不能用（瀏覽器的 HTTP 快取還在的話仍可能可以）

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

## 新增題目

在 `problems/` 下開一個資料夾，再更新 `problems/problems.json`：

```
problems/0004-xxx/
  problem.json    {"title": "...", "time_limit_ms": 1000, "version": "2026-10-08"}
  statement.md    題目敘述（Markdown，不支援 LaTeX）；「## 提示」段落會收合顯示
  tests/
    sample1.in / sample1.out   sample 開頭的會顯示在題目裡
    01.in / 01.out
```

```sh
go test ./problems -update   # 從每題的 problem.json 重新產生 problems.json
```

`version` 是題目最後修改的日期（`YYYY-MM-DD`），改了敘述、測資或時間限制就要更新；
提交紀錄會記下評測時的版本，版本不同時標示「舊版」。

`problems.json` 沒更新、或 `version` 不是日期的話 `go test` 會失敗。其他來源也要在資料夾根目錄放 `problems.json`：

```json
[{"id": "0001-a-plus-b", "title": "A + B", "time_limit_ms": 1000, "version": "2026-10-06"}]
```

推到來源的分支後，使用者下次開啟時就會拿到，不用重新 build。

## 開發

需要 Go 1.27.1+ 與 Node（用 `npm pack` 抓 Pyodide、clang、WASI shim，並用 Node 跑 clang 產生 PCH）。

```sh
go test ./...
scripts/build.sh          # 輸出到 dist/
scripts/build.sh serve    # http://localhost:3000
```

`build.sh` 是包一層 toolgui 的 `go tool toolgui-wasm build|serve`，多做的事是把
Pyodide、WASI shim 與 `web/` 準備到 `.cache/assets`，並把 clang 與 PCH 放到 `dist/cpp/`。跑過一次 `build.sh` 後，也可以直接用：

```sh
go tool toolgui-wasm serve -o dist -assets .cache/assets ./cmd/offline-judge
```

直接 serve 時不帶 `-assets` 的話，頁面能開，但沒有 Python / C++ / JavaScript 環境可以執行；
`dist/cpp/` 要先由 `build.sh` 產生，C++ 才能用。

`dist/` 是靜態網站，可直接放到 GitHub Pages（見 `.github/workflows/pages.yml`，
需在 repo 設定把 Pages 來源設為 GitHub Actions）。
需要 https 或 localhost（toolgui 的 OPFS 需要 secure context）。

可安裝成 web app（PWA）：manifest 在 `pwa/manifest.json`，圖示在 `pwa/icons/`（build 時複製到 `assets/icons/`），
`pwa/head.html` 會插進 `index.html` 的 `<head>`。
