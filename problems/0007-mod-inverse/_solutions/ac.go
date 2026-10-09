package main

import "fmt"

func main() {
	var p, a int64
	fmt.Scan(&p, &a)
	r := int64(1)
	for e := p - 2; e > 0; e >>= 1 {
		if e&1 == 1 {
			r = r * a % p
		}
		a = a * a % p
	}
	fmt.Println(r)
}
