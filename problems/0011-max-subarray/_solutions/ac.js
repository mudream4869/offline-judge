function maxSubarray(a) {
  let best = a[0]
  let cur = a[0]
  for (let i = 1; i < a.length; i++) {
    cur = Math.max(a[i], cur + a[i])
    best = Math.max(best, cur)
  }
  return best
}

module.exports = { maxSubarray }
