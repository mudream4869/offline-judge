// Accepts any value with the highest count.
export default function check(input, output) {
  const count = new Map()
  let best = 0
  for (const x of input.trim().split(/\s+/).slice(1)) {
    const n = (count.get(x) ?? 0) + 1
    count.set(x, n)
    best = Math.max(best, n)
  }

  const toks = output.trim().split(/\s+/)
  if (toks.length !== 1 || !/^-?\d+$/.test(toks[0])) return '輸出應為一個整數'
  const got = count.get(BigInt(toks[0]).toString()) ?? 0
  if (got !== best) return `${toks[0]} 出現 ${got} 次，最多的出現 ${best} 次`
  return true
}
