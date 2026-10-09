// Trial division up to sqrt(n).
package main

import "fmt"

func main() {
	var n int64
	fmt.Scan(&n)
	r := n
	for p := int64(2); p*p <= n; p++ {
		if n%p != 0 {
			continue
		}
		for n%p == 0 {
			n /= p
		}
		r -= r / p
	}
	if n > 1 {
		r -= r / n
	}
	fmt.Println(r)
}
