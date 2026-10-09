const [p, a] = require('fs').readFileSync(0, 'utf8').trim().split(/\s+/).map(BigInt)
let r = 1n
for (let e = p - 2n, b = a; e; e >>= 1n, b = b * b % p) if (e & 1n) r = r * b % p
console.log(String(r))
