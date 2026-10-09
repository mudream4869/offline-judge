// O(n log n): count i with gcd(i, n) = 1.
const n = Number(require('fs').readFileSync(0, 'utf8').trim())
const gcd = (a, b) => { while (b) [a, b] = [b, a % b]; return a }
let c = 0
for (let i = 1; i <= n; i++) if (gcd(i, n) === 1) c++
console.log(c)
