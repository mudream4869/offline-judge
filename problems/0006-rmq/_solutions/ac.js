// Iterative segment tree.
const data = require('fs').readFileSync(0, 'utf8').split(/\s+/)
let pos = 0
const n = +data[pos++], q = +data[pos++]
let size = 1
while (size < n) size *= 2
const t = new Float64Array(2 * size).fill(Infinity)
for (let i = 0; i < n; i++) t[size + i] = +data[pos++]
for (let i = size - 1; i > 0; i--) t[i] = Math.min(t[2 * i], t[2 * i + 1])
const out = []
for (let k = 0; k < q; k++) {
  const op = data[pos++], a = +data[pos++], b = +data[pos++]
  if (op === 'set') {
    let i = a - 1 + size
    t[i] = b
    for (i >>= 1; i; i >>= 1) t[i] = Math.min(t[2 * i], t[2 * i + 1])
  } else {
    let res = Infinity
    for (let l = a - 1 + size, r = b + size; l < r; l >>= 1, r >>= 1) {
      if (l & 1) res = Math.min(res, t[l++])
      if (r & 1) res = Math.min(res, t[--r])
    }
    out.push(res)
  }
}
console.log(out.join('\n'))
