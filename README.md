# Offline Judge

完全在瀏覽器裡執行的 Python 解題系統，不需要後端。

- UI、題目、評測邏輯：Go，用 [toolgui](https://github.com/voilelab/toolgui) 編成 wasm
- 執行 Python：[Pyodide](https://pyodide.org/)，跑在獨立的 module worker，超時直接 `terminate()`，並預先暖機一個備用 worker
- 測資不保密，題目與測資直接嵌在 wasm 裡

## 架構

```
page ── toolgui worker (Go wasm：UI / 題目 / 比對)
            └── pyworker.mjs (Pyodide：收 code + stdin，回 stdout / stderr / 耗時)
```

worker 介面固定為：

```
in:  {id, code, stdin}
out: {type: "ready"} | {type: "error", error}
     {type: "result", id, status: "ok"|"re", stdout, stderr, ms, fatal}
```

之後加其他語言，只要再寫一個同介面的 worker。

## 判定

| 結果 | 條件 |
| --- | --- |
| AC | 輸出相符（忽略行尾空白與結尾空行） |
| WA | 輸出不符 |
| RE | 例外或非零 `SystemExit` |
| TLE | 耗時超過限制；超過限制 +1 秒仍未結束就砍掉 worker |
| SKIP | 第一筆 TLE 之後的測資不再執行（每次 TLE 都要重載 Pyodide） |

尚未支援 MLE。

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

需要 Go 1.27.1+ 與 Node（用 `npm pack` 抓 Pyodide）。

```sh
go test ./...
scripts/build.sh          # 輸出到 dist/
scripts/build.sh serve    # http://localhost:3000
```

`build.sh` 是包一層 toolgui 的 `go tool toolgui-wasm build|serve`，多做的事是把
Pyodide 與 `web/` 準備到 `.cache/assets`。跑過一次 `build.sh` 後，也可以直接用：

```sh
go tool toolgui-wasm serve -o dist -assets .cache/assets ./cmd/offline-judge
```

直接 serve 時不帶 `-assets` 的話，頁面能開，但沒有 Python 環境可以執行。

`dist/` 是靜態網站，可直接放到 GitHub Pages（見 `.github/workflows/pages.yml`，
需在 repo 設定把 Pages 來源設為 GitHub Actions）。
需要 https 或 localhost（toolgui 的 OPFS 需要 secure context）。
