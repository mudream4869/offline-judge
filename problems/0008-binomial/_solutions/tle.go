// O(nk) Pascal's triangle.
package main

import "fmt"

const M = 1_000_000_007

func main() {
	var n, k int
	fmt.Scan(&n, &k)
	c := make([]int32, k+1)
	c[0] = 1
	for i := 1; i <= n; i++ {
		for j := min(i, k); j > 0; j-- {
			c[j] = (c[j] + c[j-1]) % M
		}
	}
	fmt.Println(c[k])
}
