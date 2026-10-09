// Trial division up to sqrt(n); n < 2^53 fits in a double.
let n = Number(require('fs').readFileSync(0, 'utf8').trim())
let r = n
for (let p = 2; p * p <= n; p++) {
  if (n % p) continue
  while (n % p === 0) n /= p
  r -= r / p
}
if (n > 1) r -= r / n
console.log(r)
