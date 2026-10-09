# Trial division up to sqrt(n).
n = int(input())
r, p = n, 2
while p * p <= n:
    if n % p == 0:
        while n % p == 0:
            n //= p
        r -= r // p
    p += 1
if n > 1:
    r -= r // n
print(r)
