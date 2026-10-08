// Runs a compiled WASI module. Cheap to start, so the Go side kills it on
// timeout and uses a spare.
//
// in:  {id, module, stdin, interactor?}
// out: {type: "ready"}
//      {type: "result", id, status: "ok"|"re", stdout, stderr, ms, fatal,
//       judged?, iaError?}
//
// With interactor (interactor.js source), stdin is its input and the
// program reads what it answers; stdout is the transcript (sandbox.mjs).

import { WASI, File, OpenFile, ConsoleStdout, Fd, wasi } from './wasi/index.js'
import { lockdown, importDefault, Interaction, interactionResult } from './sandbox.mjs'

const enc = new TextEncoder()

function collector() {
  const parts = []
  let taken = 0
  const dec = new TextDecoder()
  const fd = new ConsoleStdout((b) => parts.push(b.slice()))
  fd.text = () => {
    const d = new TextDecoder()
    return parts.map((b) => d.decode(b, { stream: true })).join('') + d.decode()
  }
  // take returns what was written since the last take.
  fd.take = () => {
    const s = parts.slice(taken).map((b) => dec.decode(b, { stream: true })).join('')
    taken = parts.length
    return s
  }
  return fd
}

// InteractiveStdin is fd 0 reading what ia answers.
class InteractiveStdin extends Fd {
  constructor(ia, out) {
    super()
    this.ia = ia
    this.out = out
    this.pending = new Uint8Array()
  }

  fd_fdstat_get() {
    return { ret: 0, fdstat: new wasi.Fdstat(wasi.FILETYPE_CHARACTER_DEVICE, 0) }
  }

  fd_read(size) {
    if (!this.pending.length) {
      const r = this.ia.read(this.out.take())
      if (r === null) return { ret: 0, data: new Uint8Array() }
      this.pending = enc.encode(r)
    }
    const data = this.pending.slice(0, size)
    this.pending = this.pending.subarray(data.length)
    return { ret: 0, data }
  }
}

self.onmessage = async ({ data: { id, module, stdin, interactor } }) => {
  const out = collector()
  const err = collector()
  let ia = null
  if (interactor) {
    lockdown()
    try {
      ia = new Interaction(await importDefault(interactor, 'interactor.js'), stdin)
    } catch (e) {
      self.postMessage({ type: 'result', id, status: 'ok', stdout: '', stderr: '', ms: 0,
        fatal: false, iaError: String(e?.stack || e) })
      return
    }
  }
  const in0 = ia ? new InteractiveStdin(ia, out) : new OpenFile(new File(enc.encode(stdin)))
  const w = new WASI(['main'], [], [in0, out, err])
  let status = 'ok'
  let t0 = performance.now()
  try {
    const inst = await WebAssembly.instantiate(module, { wasi_snapshot_preview1: w.wasiImport })
    t0 = performance.now()
    const code = w.start(inst)
    if (code !== 0) {
      status = 're'
      err.write(enc.encode(`\nexit code ${code}\n`))
    }
  } catch (e) {
    // A trap: abort(), out of bounds, stack overflow, ...
    status = 're'
    err.write(enc.encode(`\n${e}\n`))
  }
  let ms = performance.now() - t0
  let extra = { stdout: out.text() }
  if (ia) {
    extra = interactionResult(ia, out.take())
    ms -= ia.ms
  }
  self.postMessage({ type: 'result', id, status, stderr: err.text(), ms, fatal: false, ...extra })
}

self.postMessage({ type: 'ready' })
