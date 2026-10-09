// O(p): x = 1, 2, ... keeping a * x mod p (fits in a double).
const [p, a] = require('fs').readFileSync(0, 'utf8').trim().split(/\s+/).map(Number)
let x = 1, r = a
while (r !== 1) {
  x++
  r += a
  if (r >= p) r -= p
}
console.log(x)
