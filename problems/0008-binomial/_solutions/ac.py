# O(k) products, one inverse.
M = 10**9 + 7
n, k = map(int, input().split())
k = min(k, n - k)
num = den = 1
for i in range(k):
    num = num * (n - i) % M
    den = den * (i + 1) % M
print(num * pow(den, M - 2, M) % M)
