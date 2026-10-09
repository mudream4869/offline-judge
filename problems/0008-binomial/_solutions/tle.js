// O(nk) Pascal's triangle.
const M = 1000000007
const [n, k] = require('fs').readFileSync(0, 'utf8').trim().split(/\s+/).map(Number)
const c = new Int32Array(k + 1)
c[0] = 1
for (let i = 1; i <= n; i++)
  for (let j = Math.min(i, k); j > 0; j--) c[j] = (c[j] + c[j - 1]) % M
console.log(c[k])
