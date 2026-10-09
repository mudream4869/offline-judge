# O(n log n): count i with gcd(i, n) = 1.
from math import gcd

n = int(input())
print(sum(1 for i in range(1, n + 1) if gcd(i, n) == 1))
