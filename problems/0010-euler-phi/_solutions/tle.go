// O(n log n): count i with gcd(i, n) = 1.
package main

import "fmt"

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func main() {
	var n, c int64
	fmt.Scan(&n)
	for i := int64(1); i <= n; i++ {
		if gcd(i, n) == 1 {
			c++
		}
	}
	fmt.Println(c)
}
