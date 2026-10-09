package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
)

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	in.Split(bufio.ScanWords)
	next := func() int {
		in.Scan()
		x, _ := strconv.Atoi(in.Text())
		return x
	}
	a := make([]int, next())
	for i := range a {
		a[i] = next()
	}
	fmt.Println(maxSubarray(a))
}
