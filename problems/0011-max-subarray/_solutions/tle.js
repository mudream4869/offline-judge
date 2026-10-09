function maxSubarray(a) {
  let best = a[0]
  for (let i = 0; i < a.length; i++) {
    let s = 0
    for (let j = i; j < a.length; j++) {
      s += a[j]
      if (s > best) best = s
    }
  }
  return best
}

module.exports = { maxSubarray }
