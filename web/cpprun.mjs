// Runs a compiled WASI module. Cheap to start, so the Go side kills it on
// timeout and uses a spare.
//
// in:  {id, module, stdin}
// out: {type: "ready"}
//      {type: "result", id, status: "ok"|"re", stdout, stderr, ms, fatal}

import { WASI, File, OpenFile, ConsoleStdout } from './wasi/index.js'

const enc = new TextEncoder()

function collector() {
  const parts = []
  const fd = new ConsoleStdout((b) => parts.push(b))
  fd.text = () => {
    const dec = new TextDecoder()
    return parts.map((b) => dec.decode(b, { stream: true })).join('') + dec.decode()
  }
  return fd
}

self.onmessage = async ({ data: { id, module, stdin } }) => {
  const out = collector()
  const err = collector()
  const wasi = new WASI(['main'], [], [new OpenFile(new File(enc.encode(stdin))), out, err])
  let status = 'ok'
  let t0 = performance.now()
  try {
    const inst = await WebAssembly.instantiate(module, { wasi_snapshot_preview1: wasi.wasiImport })
    t0 = performance.now()
    const code = wasi.start(inst)
    if (code !== 0) {
      status = 're'
      err.write(enc.encode(`\nexit code ${code}\n`))
    }
  } catch (e) {
    // A trap: abort(), out of bounds, stack overflow, ...
    status = 're'
    err.write(enc.encode(`\n${e}\n`))
  }
  const ms = performance.now() - t0
  self.postMessage({
    type: 'result', id, status, stdout: out.text(), stderr: err.text(), ms, fatal: false,
  })
}

self.postMessage({ type: 'ready' })
