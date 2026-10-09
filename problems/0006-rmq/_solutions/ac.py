# Iterative segment tree.
import sys

data = sys.stdin.buffer.read().split()
n, q = int(data[0]), int(data[1])
size = 1
while size < n:
    size *= 2
INF = 1 << 62
t = [INF] * (2 * size)
t[size:size + n] = map(int, data[2:2 + n])
for i in range(size - 1, 0, -1):
    t[i] = min(t[2 * i], t[2 * i + 1])
out = []
pos = 2 + n
for _ in range(q):
    op, a, b = data[pos], int(data[pos + 1]), int(data[pos + 2])
    pos += 3
    if op == b"set":
        i = a - 1 + size
        t[i] = b
        i >>= 1
        while i:
            t[i] = min(t[2 * i], t[2 * i + 1])
            i >>= 1
    else:
        l, r, res = a - 1 + size, b + size, INF
        while l < r:
            if l & 1:
                res = min(res, t[l])
                l += 1
            if r & 1:
                r -= 1
                res = min(res, t[r])
            l >>= 1
            r >>= 1
        out.append(res)
print("\n".join(map(str, out)))
