// O(k) products, one inverse.
package main

import "fmt"

const M = 1_000_000_007

func power(a, e int64) int64 {
	r := int64(1)
	for ; e > 0; e >>= 1 {
		if e&1 == 1 {
			r = r * a % M
		}
		a = a * a % M
	}
	return r
}

func main() {
	var n, k int64
	fmt.Scan(&n, &k)
	k = min(k, n-k)
	num, den := int64(1), int64(1)
	for i := int64(0); i < k; i++ {
		num = num * (n - i) % M
		den = den * (i + 1) % M
	}
	fmt.Println(num * power(den, M-2) % M)
}
