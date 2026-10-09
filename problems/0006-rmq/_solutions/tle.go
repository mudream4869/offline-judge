// O(NQ) scan.
package main

import (
	"bufio"
	"fmt"
	"os"
	"slices"
)

func main() {
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	var n, q int
	fmt.Fscan(in, &n, &q)
	a := make([]int64, n)
	for i := range a {
		fmt.Fscan(in, &a[i])
	}
	var op string
	var x, y int64
	for ; q > 0; q-- {
		fmt.Fscan(in, &op, &x, &y)
		if op == "set" {
			a[x-1] = y
		} else {
			fmt.Fprintln(out, slices.Min(a[x-1:y]))
		}
	}
}
