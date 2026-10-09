const { maxSubarray } = require('./solution')

const data = require('fs').readFileSync(0, 'utf8').split(/\s+/).filter(Boolean).map(Number)
console.log(String(maxSubarray(data.slice(1, 1 + data[0]))))
