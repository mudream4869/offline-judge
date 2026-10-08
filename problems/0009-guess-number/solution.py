# Reference solution, used by problems_test.go; not part of the problem.
lo, hi = 1, int(input())
while True:
    mid = (lo + hi) // 2
    print("?", mid, flush=True)
    r = input().strip()
    if r == "=":
        break
    if r == "<":
        hi = mid - 1
    else:
        lo = mid + 1
print("!", mid, flush=True)
