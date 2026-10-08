// Runs a problem's checker.js. Checkers come from the problem source, so
// storage and network are taken away first. The Go side kills the worker
// when a check takes too long.
//
// checker.js: export default function check(input, output, answer)
//   returns true (AC), false or a message string (WA)
//
// in:  {id, checker, input, output, answer}
// out: {type: "ready"}
//      {type: "result", id, ok, message, error}

const { Blob, URL } = self

// Best effort: a checker only needs to compute.
for (const name of [
  'indexedDB', 'caches', 'fetch', 'XMLHttpRequest', 'WebSocket', 'WebTransport', 'EventSource',
  'BroadcastChannel', 'Worker', 'SharedWorker', 'importScripts', 'navigator', 'cookieStore',
]) {
  for (let o = self; o; o = Object.getPrototypeOf(o)) {
    const d = Object.getOwnPropertyDescriptor(o, name)
    if (d) {
      Object.defineProperty(o, name, { value: undefined, configurable: false, writable: false })
      break
    }
  }
}

// The last checker, so cases of one submission import it once.
let last = { src: null, fn: null }

async function load(src) {
  if (last.src !== src) {
    const url = URL.createObjectURL(new Blob([src], { type: 'text/javascript' }))
    try {
      const m = await import(url)
      if (typeof m.default !== 'function') throw new Error('checker.js 沒有 export default 函式')
      last = { src, fn: m.default }
    } finally {
      URL.revokeObjectURL(url)
    }
  }
  return last.fn
}

self.onmessage = async ({ data: { id, checker, input, output, answer } }) => {
  try {
    const r = await (await load(checker))(input, output, answer)
    if (typeof r !== 'boolean' && typeof r !== 'string') {
      throw new Error(`checker 應回傳 true / false / 字串，卻回傳了 ${typeof r}`)
    }
    self.postMessage({ type: 'result', id, ok: r === true, message: r === true ? '' : String(r || '') })
  } catch (e) {
    self.postMessage({ type: 'result', id, ok: false, message: '', error: String(e?.stack || e) })
  }
}

self.postMessage({ type: 'ready' })
