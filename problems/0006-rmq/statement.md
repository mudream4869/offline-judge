# 區間最小值

給定 $N$ 個整數 $a_1, a_2, \ldots, a_N$，依序處理 $Q$ 個操作：

- `set p x`：把 $a_p$ 改成 $x$。
- `getmin l r`：輸出 $\min(a_l, a_{l+1}, \ldots, a_r)$。

## 輸入

第一行兩個整數 $N$、$Q$（$1 \le N, Q \le 5 \times 10^4$）。

第二行 $N$ 個整數 $a_i$（$|a_i| \le 10^9$）。

接下來 $Q$ 行，每行一個操作，格式如上（$1 \le p \le N$，$|x| \le 10^9$，$1 \le l \le r \le N$）。保證至少有一個 `getmin`。

## 輸出

每個 `getmin` 輸出一行，為該區間的最小值。

## 提示

每次都掃過整個區間是 $O(NQ)$，只能通過子任務 1。用線段樹（segment tree）可以讓兩種操作都在 $O(\log N)$ 完成。
