# 最大子陣列和

給一個長度為 $N$ 的整數陣列 $a$，求和最大的**非空**連續子陣列的和。

這是**函式題**：不用處理輸入輸出，只要實作下面的函式，題目的 grader 會讀入資料、呼叫它並輸出回傳值。

| 語言 | 函式 |
| --- | --- |
| Python | `def max_subarray(a: list[int]) -> int` |
| C++ | `long long max_subarray(const std::vector<int>& a)` |
| JavaScript | `function maxSubarray(a)`，並 `module.exports = { maxSubarray }` |
| Go | `func maxSubarray(a []int) int` |

編輯器的預設程式碼已經是這個函式的空殼。

## 輸入（grader 讀入）

第一行是 $N$，第二行是 $a_1, a_2, \ldots, a_N$。

## 輸出（grader 輸出）

函式的回傳值。

## 限制

- $1 \le N \le 2 \times 10^5$
- $|a_i| \le 10^9$

## 提示

從左到右掃過陣列，記住「以目前位置結尾」的最大和。
