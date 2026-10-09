# O(NQ): min over a slice.
import sys

data = sys.stdin.buffer.read().split()
n, q = int(data[0]), int(data[1])
a = list(map(int, data[2:2 + n]))
out = []
pos = 2 + n
for _ in range(q):
    op, x, y = data[pos], int(data[pos + 1]), int(data[pos + 2])
    pos += 3
    if op == b"set":
        a[x - 1] = y
    else:
        out.append(min(a[x - 1:y]))
print("\n".join(map(str, out)))
