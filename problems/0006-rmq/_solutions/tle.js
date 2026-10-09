// O(NQ) scan.
const data = require('fs').readFileSync(0, 'utf8').split(/\s+/)
let pos = 0
const n = +data[pos++], q = +data[pos++]
const a = new Float64Array(n)
for (let i = 0; i < n; i++) a[i] = +data[pos++]
const out = []
for (let k = 0; k < q; k++) {
  const op = data[pos++], x = +data[pos++], y = +data[pos++]
  if (op === 'set') {
    a[x - 1] = y
  } else {
    let res = Infinity
    for (let i = x - 1; i < y; i++) if (a[i] < res) res = a[i]
    out.push(res)
  }
}
console.log(out.join('\n'))
