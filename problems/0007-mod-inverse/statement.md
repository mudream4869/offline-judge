# 模反元素

給定質數 $p$ 與 $Q$ 個整數 $a_1, a_2, \ldots, a_Q$，對每個 $a_i$ 求它在模 $p$ 下的反元素，
也就是滿足 $a_i \cdot x \equiv 1 \pmod{p}$ 且 $0 \le x < p$ 的整數 $x$。

## 輸入

第一行兩個整數 $p$、$Q$（$2 \le p \le 1.1 \times 10^9$，$p$ 是質數；$1 \le Q \le 5 \times 10^4$）。

接下來 $Q$ 行，每行一個整數 $a_i$（$1 \le a_i < p$）。

## 輸出

輸出 $Q$ 行，第 $i$ 行為 $a_i$ 的反元素。

## 提示

由費馬小定理，$a^{p-1} \equiv 1 \pmod{p}$，所以 $a^{-1} \equiv a^{p-2} \pmod{p}$，
用快速冪可以在 $O(\log p)$ 算出。也可以用擴展歐幾里得算法。

兩個小於 $p$ 的數相乘可能超過 $2^{53}$，JavaScript 要用 `BigInt`；C++ 要用 `long long`。
