# Offline Judge

完全在瀏覽器裡執行的 Python / C++ 解題系統，不需要後端。

- UI、題目、評測邏輯：Go，用 [toolgui](https://github.com/voilelab/toolgui) 編成 wasm
- 執行 Python：[Pyodide](https://pyodide.org/)，跑在獨立的 module worker，超時直接 `terminate()`，並預先暖機一個備用 worker
- 執行 C++（PoC）：[YoWASP clang](https://yowasp.org/) 編成 WASI wasm，再用
  [browser_wasi_shim](https://github.com/bjorn3/browser_wasi_shim) 在可砍掉的 worker 裡執行
- 測資不保密，題目與測資直接嵌在 wasm 裡

## 架構

```
page ── toolgui worker (Go wasm：UI / 題目 / 比對)
            ├── pyworker.mjs   (Pyodide：收 code + stdin，回 stdout / stderr / 耗時)
            ├── cppcompile.mjs (clang：收 code，回 WebAssembly.Module 或編譯錯誤；常駐)
            └── cpprun.mjs     (收 module + stdin，回 stdout / stderr / 耗時)
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
| RE | 例外、非零 `SystemExit` 或非零 exit code |
| TLE | 耗時超過限制；超過限制 +1 秒仍未結束就砍掉 worker |
| SKIP | 第一筆 TLE 之後的測資不再執行（每次 TLE 都要重載 Pyodide） |

尚未支援 MLE。

## C++ 的限制（PoC）

- 編譯參數：`-std=c++17 -O2`，stack 64 MB
- `<bits/stdc++.h>` 是自己寫的（`web/stdc++.h`），只含常用標頭；在 build 時預先編成 PCH，
  有 include 它的程式才會用到
- wasi 版 libc++ 不支援例外：`throw` 直接 abort 判 RE，`catch` 不會被執行
- 執行速度約為原生的 1/3，時間限制目前沒有依語言調整
- 第一次選 C++ 時才下載 clang（gzip 後約 27 MB）加上 PCH（約 10 MB）；編譯一次約 1 秒
- clang 與 PCH 放在 `dist/cpp/`，不在 `assets/` 裡，所以 `-offline` 的 service worker
  不會預先快取它們：C++ 離線時不能用（瀏覽器的 HTTP 快取還在的話仍可能可以）

## 新增題目

在 `problems/` 下開一個資料夾：

```
problems/0004-xxx/
  problem.json    {"title": "...", "time_limit_ms": 1000}
  statement.md    題目敘述（Markdown，不支援 LaTeX）
  tests/
    sample1.in / sample1.out   sample 開頭的會顯示在題目裡
    01.in / 01.out
```

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

直接 serve 時不帶 `-assets` 的話，頁面能開，但沒有 Python / C++ 環境可以執行；
`dist/cpp/` 要先由 `build.sh` 產生，C++ 才能用。

`dist/` 是靜態網站，可直接放到 GitHub Pages（見 `.github/workflows/pages.yml`，
需在 repo 設定把 Pages 來源設為 GitHub Actions）。
需要 https 或 localhost（toolgui 的 OPFS 需要 secure context）。
