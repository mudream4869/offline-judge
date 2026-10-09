// O(k) products, one inverse.
const M = 1000000007n
const [n, k0] = require('fs').readFileSync(0, 'utf8').trim().split(/\s+/).map(BigInt)
const k = k0 < n - k0 ? k0 : n - k0
let num = 1n, den = 1n
for (let i = 0n; i < k; i++) {
  num = num * (n - i) % M
  den = den * (i + 1n) % M
}
let inv = 1n
for (let e = M - 2n, b = den; e; e >>= 1n, b = b * b % M) if (e & 1n) inv = inv * b % M
console.log(String(num * inv % M))
