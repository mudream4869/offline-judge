# O(p): x = 1, 2, ... keeping a * x mod p.
p, a = map(int, input().split())
x, r = 1, a
while r != 1:
    x += 1
    r += a
    if r >= p:
        r -= p
print(x)
