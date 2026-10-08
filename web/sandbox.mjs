// Runs code that comes with a problem (checker.js, interactor.js). It comes
// from the problem source, so storage and network are taken away first.

const { Blob, URL } = self

// lockdown removes what problem code has no use for. Best effort: a module's
// own imports can still reach the network.
export function lockdown() {
  for (const name of [
    'indexedDB', 'caches', 'fetch', 'XMLHttpRequest', 'WebSocket', 'WebTransport', 'EventSource',
    'BroadcastChannel', 'Worker', 'SharedWorker', 'importScripts', 'navigator', 'cookieStore',
  ]) {
    for (let o = self; o; o = Object.getPrototypeOf(o)) {
      const d = Object.getOwnPropertyDescriptor(o, name)
      if (d) {
        if (d.configurable) {
          Object.defineProperty(o, name, { value: undefined, configurable: false, writable: false })
        }
        break
      }
    }
  }
}

// The last import, so the cases of one submission import it once.
let last = { src: null, fn: null }

// importDefault returns the default export of module source src, a function.
export async function importDefault(src, name) {
  if (last.src !== src) {
    const url = URL.createObjectURL(new Blob([src], { type: 'text/javascript' }))
    try {
      const m = await import(url)
      if (typeof m.default !== 'function') throw new Error(`${name} 沒有 export default 函式`)
      last = { src, fn: m.default }
    } finally {
      URL.revokeObjectURL(url)
    }
  }
  return last.fn
}

// verdict turns what a checker or interactor returned into {ok, message}.
export function verdict(r, name) {
  if (typeof r !== 'boolean' && typeof r !== 'string') {
    throw new Error(`${name} 應回傳 true / false / 字串，卻回傳了 ${typeof r}`)
  }
  return { ok: r === true, message: r === true ? '' : String(r || '') }
}

// Interaction drives interactor.js for one run and keeps a transcript.
//
// interactor.js: export default function (input) { return { read(out), finish(out) } }
//   read(out):   the program wants input; out is what it wrote since the
//                last call. Returns what it reads next, or null for EOF.
//   finish(out): the program ended; out is the rest of its output.
//                Returns true (AC), false or a message string (WA).
//
// Errors thrown by the interactor are kept in error, not passed to the
// program: they are the problem's fault.
export class Interaction {
  constructor(make, input) {
    this.log = []
    this.ms = 0 // time spent in the interactor
    this.eof = false
    this.error = null
    try {
      this.ia = make(input)
    } catch (e) {
      this.fail(e)
    }
  }

  read(out) {
    this.note('→', out)
    if (this.eof) return null
    const t0 = performance.now()
    let r = null
    try {
      r = this.ia.read(out)
    } catch (e) {
      this.fail(e)
    }
    this.ms += performance.now() - t0
    if (r == null || r === '') {
      this.eof = true
      return null
    }
    r = String(r)
    this.note('←', r)
    return r
  }

  // finish returns {ok, message}, or null after an interactor error.
  finish(out) {
    this.note('→', out)
    if (this.error) return null
    const t0 = performance.now()
    try {
      return verdict(this.ia.finish(out), 'interactor.js 的 finish')
    } catch (e) {
      this.fail(e)
      return null
    } finally {
      this.ms += performance.now() - t0
    }
  }

  transcript() {
    return this.log.join('')
  }

  fail(e) {
    this.eof = true
    this.error ??= String(e?.stack || e)
    this.ia ??= { read: () => null, finish: () => false }
  }

  // note adds s to the transcript, each line marked with dir.
  note(dir, s) {
    if (!s) return
    for (const line of s.replace(/\n$/, '').split('\n')) this.log.push(`${dir} ${line}\n`)
  }
}

// interactionResult is the part of a worker result an Interaction adds.
export function interactionResult(ia, rest) {
  const judged = ia.finish(rest)
  return { stdout: ia.transcript(), judged, iaError: ia.error }
}
