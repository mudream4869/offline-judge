// Iterative segment tree.
package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
)

func main() {
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	var n, q int
	fmt.Fscan(in, &n, &q)
	size := 1
	for size < n {
		size *= 2
	}
	t := make([]int64, 2*size)
	for i := range t {
		t[i] = math.MaxInt64
	}
	for i := 0; i < n; i++ {
		fmt.Fscan(in, &t[size+i])
	}
	for i := size - 1; i > 0; i-- {
		t[i] = min(t[2*i], t[2*i+1])
	}
	var op string
	var a, b int64
	for ; q > 0; q-- {
		fmt.Fscan(in, &op, &a, &b)
		if op == "set" {
			i := int(a) - 1 + size
			t[i] = b
			for i >>= 1; i > 0; i >>= 1 {
				t[i] = min(t[2*i], t[2*i+1])
			}
		} else {
			res := int64(math.MaxInt64)
			for l, r := int(a)-1+size, int(b)+size; l < r; l, r = l>>1, r>>1 {
				if l&1 == 1 {
					res = min(res, t[l])
					l++
				}
				if r&1 == 1 {
					r--
					res = min(res, t[r])
				}
			}
			fmt.Fprintln(out, res)
		}
	}
}
