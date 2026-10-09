package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	var lo, hi, mid int64 = 1, 0, 0
	fmt.Fscan(in, &hi)
	var r string
	for {
		mid = (lo + hi) / 2
		fmt.Fprintln(out, "?", mid)
		out.Flush()
		fmt.Fscan(in, &r)
		if r == "=" {
			break
		}
		if r == "<" {
			hi = mid - 1
		} else {
			lo = mid + 1
		}
	}
	fmt.Fprintln(out, "!", mid)
	out.Flush()
}
