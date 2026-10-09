# O(nk) Pascal's triangle.
M = 10**9 + 7
n, k = map(int, input().split())
c = [1] + [0] * k
for i in range(1, n + 1):
    for j in range(min(i, k), 0, -1):
        c[j] = (c[j] + c[j - 1]) % M
print(c[k])
