package main

func maxSubarray(a []int) int {
	best, cur := a[0], a[0]
	for _, x := range a[1:] {
		cur = max(x, cur+x)
		best = max(best, cur)
	}
	return best
}
