package main

func maxSubarray(a []int) int {
	best := a[0]
	for i := range a {
		s := 0
		for j := i; j < len(a); j++ {
			s += a[j]
			best = max(best, s)
		}
	}
	return best
}
