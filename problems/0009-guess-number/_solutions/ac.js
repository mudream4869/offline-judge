// readline: each answer arrives as a 'line' event.
const rl = require('readline').createInterface({ input: process.stdin })
let lo = 1
let hi = 0
let mid = 0
rl.on('line', (line) => {
  if (hi === 0) {
    hi = Number(line)
  } else if (line === '=') {
    console.log('!', mid)
    rl.close()
    return
  } else if (line === '<') {
    hi = mid - 1
  } else {
    lo = mid + 1
  }
  mid = Math.floor((lo + hi) / 2)
  console.log('?', mid)
})
