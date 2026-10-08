// Guess the number: input "N x"; the program asks "? y" and answers "! y".
export default function interact(input) {
  const [n, x] = input.trim().split(/\s+/).map(Number)
  const limit = 30
  let buf = ''
  let started = false
  let queries = 0
  let verdict = null // set once the program answers or breaks the rules

  // lines returns the complete lines in out, keeping the rest in buf.
  function lines(out) {
    const ls = (buf + out).split('\n')
    buf = ls.pop()
    return ls.map((l) => l.trim()).filter((l) => l)
  }

  // handle returns the reply to one line, or null if there is none.
  function handle(line) {
    const m = /^([?!])\s+(-?\d+)$/.exec(line)
    if (!m) {
      verdict = `無法解析的輸出：${line}`
      return null
    }
    const y = Number(m[2])
    if (m[1] === '!') {
      verdict = y === x ? true : `答案 ${y} 錯誤，正確是 ${x}`
      return null
    }
    if (++queries > limit) {
      verdict = `詢問超過 ${limit} 次`
      return null
    }
    if (y < 1 || y > n) {
      verdict = `詢問 ${y} 超出範圍 1～${n}`
      return null
    }
    return y === x ? '=' : x < y ? '<' : '>'
  }

  return {
    read(out) {
      let reply = ''
      if (!started) {
        started = true
        reply = `${n}\n`
      }
      for (const line of lines(out)) {
        if (verdict !== null) break
        const r = handle(line)
        if (r !== null) reply += r + '\n'
      }
      if (verdict !== null) return null
      if (reply === '') {
        verdict = '程式在等待回答，但沒有輸出完整的詢問（記得 flush）'
        return null
      }
      return reply
    },
    finish(out) {
      for (const line of lines(out + '\n')) {
        if (verdict !== null) break
        handle(line)
      }
      return verdict ?? '程式沒有輸出答案'
    },
  }
}
