// O(p): x = 1, 2, ... keeping a * x mod p.
package main

import "fmt"

func main() {
	var p, a int64
	fmt.Scan(&p, &a)
	x, r := int64(1), a
	for r != 1 {
		x++
		r += a
		if r >= p {
			r -= p
		}
	}
	fmt.Println(x)
}
