// Runs JavaScript submissions with a minimal Node-like environment: require
// ('fs', 'readline'), process.stdin/stdout, console. One worker per run
// (fatal: true), so globals don't leak between runs.
//
// in:  {id, code, stdin, outputLimit?}  (outputLimit: stdout + stderr)
// out: {type: "ready"}
//      {type: "result", id, status: "ok"|"re"|"ole", stdout, stderr, ms, fatal}

const realSetTimeout = setTimeout
const realClearTimeout = clearTimeout
const realSetInterval = setInterval
const realClearInterval = clearInterval
const enc = new TextEncoder()
const dec = new TextDecoder()

// ---- console.log formatting (a subset of Node's util.format) ----

function inspect(v, depth = 0, seen = []) {
  switch (typeof v) {
    case 'string': return depth ? `'${v.replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/\n/g, '\\n')}'` : v
    case 'number': return Object.is(v, -0) ? '-0' : String(v)
    case 'bigint': return `${v}n`
    case 'symbol': return v.toString()
    case 'function': return `[Function: ${v.name || '(anonymous)'}]`
    case 'undefined': return 'undefined'
  }
  if (v === null) return 'null'
  if (typeof v !== 'object') return String(v)
  if (seen.includes(v)) return '[Circular]'
  if (v instanceof Error) return v.stack || String(v)
  if (v instanceof Date) return v.toISOString()
  if (v instanceof RegExp) return String(v)
  if (depth > 2) return Array.isArray(v) ? '[Array]' : '[Object]'
  const sub = (x) => inspect(x, depth + 1, [...seen, v])
  const list = (open, items, close) => (items.length ? `${open} ${items.join(', ')} ${close}` : `${open}${close}`)
  const more = (n) => (n > 100 ? [`... ${n - 100} more items`] : [])
  if (Array.isArray(v) || ArrayBuffer.isView(v)) {
    const items = Array.from(v.slice(0, 100), sub).concat(more(v.length))
    const name = Array.isArray(v) ? '' : `${v.constructor.name}(${v.length}) `
    return name + list('[', items, ']')
  }
  if (v instanceof Map) {
    const items = [...v].slice(0, 100).map(([k, x]) => `${sub(k)} => ${sub(x)}`).concat(more(v.size))
    return `Map(${v.size}) ` + list('{', items, '}')
  }
  if (v instanceof Set) {
    return `Set(${v.size}) ` + list('{', [...v].slice(0, 100).map(sub).concat(more(v.size)), '}')
  }
  const key = (k) => (/^[A-Za-z_$][\w$]*$/.test(k) ? k : sub(k))
  const items = Object.keys(v).map((k) => `${key(k)}: ${sub(v[k])}`)
  const proto = Object.getPrototypeOf(v)
  const name = proto && proto !== Object.prototype && proto.constructor ? proto.constructor.name + ' ' : ''
  return name + list('{', items, '}')
}

function format(args) {
  if (typeof args[0] === 'string' && args[0].includes('%')) {
    let i = 1
    const head = args[0].replace(/%([sdifjoOc%])/g, (m, c) => {
      if (c === '%') return '%'
      if (i >= args.length) return m
      const a = args[i++]
      switch (c) {
        case 's': return typeof a === 'string' ? a : inspect(a, 1)
        case 'd': return typeof a === 'bigint' ? `${a}n` : String(Number(a))
        case 'i': return typeof a === 'bigint' ? `${a}n` : String(parseInt(a))
        case 'f': return String(parseFloat(a))
        case 'j': return JSON.stringify(a)
        case 'c': return ''
        default: return inspect(a, 1)
      }
    })
    return [head, ...args.slice(i).map((a) => inspect(a))].join(' ')
  }
  return args.map((a) => inspect(a)).join(' ')
}

// ---- stdin ----

class Buffer extends Uint8Array {
  toString() { return dec.decode(this) }
  static from(s) { return new Buffer(typeof s === 'string' ? enc.encode(s) : s) }
}

class Emitter {
  constructor() { this.ls = {} }
  on(ev, fn) { (this.ls[ev] ||= []).push({ fn }); return this }
  once(ev, fn) { (this.ls[ev] ||= []).push({ fn, once: true }); return this }
  off(ev, fn) { this.ls[ev] = (this.ls[ev] || []).filter((l) => l.fn !== fn); return this }
  emit(ev, ...args) {
    const ls = this.ls[ev] || []
    this.ls[ev] = ls.filter((l) => !l.once)
    for (const l of ls) l.fn.apply(this, args)
    return ls.length > 0
  }
}
Emitter.prototype.addListener = Emitter.prototype.on
Emitter.prototype.removeListener = Emitter.prototype.off

class ExitSignal { constructor(code) { this.code = code } }
class OutputLimit {}

// ---- run ----

self.onmessage = async ({ data: { id, code, stdin, outputLimit = Infinity } }) => {
  const out = []
  // stdout + stderr written, in UTF-16 code units: close enough to bytes for a limit
  let outSize = 0
  const err = []
  let done = false
  let t0 = 0

  const finish = (status, extra) => {
    if (done) return
    done = true
    if (extra) err.push(extra)
    self.postMessage({
      type: 'result', id, status, stdout: out.join(''), stderr: err.join(''),
      ms: performance.now() - t0, fatal: true,
    })
  }
  // put appends s to buf (stdout or stderr), ending the run past the output limit.
  const put = (buf, s) => {
    outSize += s.length
    if (outSize > outputLimit) {
      finish('ole')
      throw new OutputLimit()
    }
    buf.push(s)
  }
  const putOut = (s) => put(out, s)
  const putErr = (s) => put(err, s)
  const fail = (e) => {
    if (e instanceof ExitSignal) {
      finish(e.code ? 're' : 'ok', e.code ? `\nexit code ${e.code}\n` : '')
    } else {
      finish('re', errText(e))
    }
  }
  const guard = (fn) => function (...args) {
    if (done) return
    try { return fn.apply(this, args) } catch (e) { fail(e) }
  }

  // process.stdin and readline interfaces get their events once the
  // current code returns, like in Node.
  const sources = []
  const addSource = (s) => { sources.push(s); settle() }

  // Timers, tracked so we know when the program is idle.
  let pending = 0
  let wake = null
  function settle() { if ((pending === 0 || sources.length) && wake) wake() }
  const timers = new Set()
  self.setTimeout = (fn, ms, ...a) => {
    pending++
    const t = realSetTimeout(() => {
      timers.delete(t); pending--
      guard(fn)(...a); settle()
    }, ms)
    timers.add(t)
    return t
  }
  self.clearTimeout = (t) => {
    if (timers.delete(t)) { realClearTimeout(t); pending--; settle() }
  }
  self.setInterval = (fn, ms, ...a) => {
    pending++
    const t = realSetInterval(guard(fn), ms, ...a)
    timers.add(t)
    return t
  }
  self.clearInterval = (t) => {
    if (timers.delete(t)) { realClearInterval(t); pending--; settle() }
  }
  self.setImmediate = (fn, ...a) => self.setTimeout(fn, 0, ...a)
  self.clearImmediate = self.clearTimeout
  self.addEventListener('unhandledrejection', (ev) => { ev.preventDefault(); fail(ev.reason) })

  const lines = stdin.split('\n').map((l) => l.replace(/\r$/, ''))
  if (lines[lines.length - 1] === '') lines.pop()

  const pstdin = new Emitter()
  let stdinEnc = null
  pstdin.fd = 0
  pstdin.setEncoding = (e) => { stdinEnc = e; return pstdin }
  pstdin.resume = pstdin.pause = () => pstdin
  addSource(() => {
    if (stdin !== '') pstdin.emit('data', stdinEnc ? stdin : Buffer.from(stdin))
    pstdin.emit('end')
    pstdin.emit('close')
  })

  const writer = (push) => ({
    write: (s, ...rest) => {
      push(typeof s === 'string' ? s : dec.decode(s))
      const cb = rest.find((x) => typeof x === 'function')
      if (cb) queueMicrotask(cb)
      return true
    },
    isTTY: false,
  })

  let exitCode = 0
  const process = {
    stdin: pstdin,
    stdout: writer(putOut),
    stderr: writer(putErr),
    argv: ['node', '/main.js'],
    env: {},
    platform: 'linux',
    version: 'v22.0.0',
    versions: { node: '22.0.0' },
    exit: (c) => { throw new ExitSignal(c ?? exitCode) },
    get exitCode() { return exitCode },
    set exitCode(c) { exitCode = c | 0 },
    hrtime: Object.assign((prev) => {
      const ns = BigInt(Math.round(performance.now() * 1e6))
      const t = [Number(ns / 1000000000n), Number(ns % 1000000000n)]
      if (!prev) return t
      let s = t[0] - prev[0], n = t[1] - prev[1]
      if (n < 0) { s--; n += 1e9 }
      return [s, n]
    }, { bigint: () => BigInt(Math.round(performance.now() * 1e6)) }),
    memoryUsage: () => ({ rss: 0, heapTotal: 0, heapUsed: 0, external: 0 }),
    nextTick: (fn, ...a) => queueMicrotask(guard(() => fn(...a))),
    on() { return process },
    once() { return process },
    cwd: () => '/',
  }

  const isStdin = (p) => p === 0 || p === '/dev/stdin'
  const fs = {
    readFileSync(p, opt) {
      if (!isStdin(p)) throw new Error(`ENOENT: no such file or directory, open '${p}'`)
      const e = typeof opt === 'string' ? opt : opt?.encoding
      return e ? stdin : Buffer.from(stdin)
    },
    writeSync(fd, s) {
      const t = typeof s === 'string' ? s : dec.decode(s)
      if (fd === 2) putErr(t)
      else putOut(t)
    },
  }

  const readline = {
    createInterface() {
      const rl = new Emitter()
      let closed = false
      rl.close = () => { if (!closed) { closed = true; rl.emit('close') } }
      rl.setPrompt = rl.prompt = rl.pause = rl.resume = () => rl
      rl[Symbol.asyncIterator] = async function* () {
        const q = []
        let end = false
        let notify = null
        rl.on('line', (l) => { q.push(l); notify?.() })
        rl.on('close', () => { end = true; notify?.() })
        for (;;) {
          if (q.length) { yield q.shift(); continue }
          if (end) return
          await new Promise((r) => { notify = r })
          notify = null
        }
      }
      addSource(() => {
        for (const l of lines) {
          if (closed) return
          rl.emit('line', l)
        }
        rl.close()
      })
      return rl
    },
  }

  const modules = {
    fs, readline, process,
    os: { EOL: '\n' },
    util: { format: (...a) => format(a), inspect: (v) => inspect(v, 1) },
  }
  const require = (name) => {
    const m = modules[name.replace(/^node:/, '')]
    if (!m) throw new Error(`Cannot find module '${name}'`)
    return m
  }

  const log = (...a) => { putOut(format(a) + '\n') }
  const elog = (...a) => { putErr(format(a) + '\n') }
  self.console = { log, info: log, debug: log, error: elog, warn: elog, trace: elog }
  self.process = process
  self.require = require
  self.Buffer = Buffer
  self.global = self

  t0 = performance.now()
  try {
    // The code starts on line 2; errText shifts line numbers back.
    const fn = (0, eval)(
      '(function (exports, require, module, __filename, __dirname) {\n' + code +
      '\n})\n//# sourceURL=main.js')
    const module = { exports: {} }
    fn.call(module.exports, module.exports, require, module, '/main.js', '/')
    // Event loop: run until no input events or timers are left.
    for (;;) {
      await new Promise((r) => realSetTimeout(r, 0))
      if (done) return
      if (sources.length) {
        guard(sources.shift())()
        continue
      }
      if (pending === 0) break
      await new Promise((r) => { wake = r })
      wake = null
    }
  } catch (e) {
    fail(e)
    return
  }
  finish(exitCode ? 're' : 'ok', exitCode ? `\nexit code ${exitCode}\n` : '')
}

// Keep the user's frames only.
function errText(e) {
  if (!(e instanceof Error)) return `Uncaught ${inspect(e, 1)}\n`
  const lines = String(e.stack || e).split('\n')
  const keep = lines.filter((l, i) => i === 0 || !/^\s+at /.test(l) || l.includes('main.js'))
  if (!keep[0].includes(e.message)) keep.unshift(String(e))
  return keep.join('\n')
    .replace(/at (Object\.)?eval \(/g, 'at (')
    .replace(/main\.js:(\d+)/g, (_, n) => `main.js:${n - 1}`) + '\n'
}

self.postMessage({ type: 'ready' })
