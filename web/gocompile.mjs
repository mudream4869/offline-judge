// Compiles Go to a WASI module with the real toolchain (cmd/compile and
// cmd/link built for wasip1). Long-lived like cppcompile.mjs; programs run in
// wasirun.mjs.
//
// in:  {id, code}
// out: {type: "ready"} | {type: "error", error}
//      {type: "result", id, status: "ok"|"ce", module, stderr, ms}

import { WASI, File, OpenFile, ConsoleStdout, PreopenDirectory } from './wasi/index.js'

const enc = new TextEncoder()

let compileMod = null
let linkMod = null
let std = null // Map of file name → File: importcfg and the .a archives

// Entries of an uncompressed ustar archive.
function untar(buf) {
  const files = new Map()
  const dec = new TextDecoder()
  const str = (o, n) => dec.decode(buf.subarray(o, o + n)).replace(/\0.*$/s, '')
  for (let o = 0; o + 512 <= buf.length && buf[o]; ) {
    const name = str(o, 100)
    const size = parseInt(str(o + 124, 12), 8)
    if (str(o + 156, 1) === '0' || str(o + 156, 1) === '') {
      files.set(name.replace(/^\.\//, ''), buf.slice(o + 512, o + 512 + size))
    }
    o += 512 + Math.ceil(size / 512) * 512
  }
  return files
}

// Runs a toolchain module on the in-memory root; returns the exit code.
function run(mod, args, root, log) {
  const out = new ConsoleStdout(log)
  const wasi = new WASI(args, [], [new OpenFile(new File([])), out, out, root], { debug: false })
  return WebAssembly.instantiate(mod, { wasi_snapshot_preview1: wasi.wasiImport })
    .then((inst) => wasi.start(inst))
}

async function compile(code) {
  let stderr = ''
  const dec = new TextDecoder()
  const log = (b) => { stderr += dec.decode(b, { stream: true }) }
  const files = new Map(std)
  files.set('main.go', new File(enc.encode(code)))
  const root = new PreopenDirectory('/', files)
  try {
    if (await run(compileMod, ['compile', '-p', 'main', '-complete', '-importcfg', 'importcfg',
      '-o', 'main.a', '-pack', 'main.go'], root, log) !== 0) {
      return { module: null, stderr }
    }
    const cfg = dec.decode(std.get('importcfg').data) + 'packagefile main=main.a\n'
    files.set('importcfg.link', new File(enc.encode(cfg)))
    if (await run(linkMod, ['link', '-importcfg', 'importcfg.link', '-s', '-w',
      '-o', 'main.wasm', 'main.a'], root, log) !== 0) {
      return { module: null, stderr }
    }
    return { module: await WebAssembly.compile(files.get('main.wasm').data), stderr }
  } catch (e) {
    return { module: null, stderr: stderr + String(e) }
  }
}

self.onmessage = async ({ data: { id, code } }) => {
  await ready
  const t0 = performance.now()
  const { module, stderr } = await compile(code)
  const ms = performance.now() - t0
  self.postMessage({ type: 'result', id, status: module ? 'ok' : 'ce', module, stderr, ms })
}

const ready = (async () => {
  try {
    // The toolchain lives outside assets/ so the service worker doesn't precache it.
    const url = (name) => new URL('../go/' + name, import.meta.url)
    const get = async (name) => {
      const resp = await fetch(url(name))
      if (!resp.ok) throw new Error(`${name}: ${resp.status}`)
      return resp
    }
    ;[compileMod, linkMod, std] = await Promise.all([
      WebAssembly.compileStreaming(get('compile.wasm')),
      WebAssembly.compileStreaming(get('link.wasm')),
      get('std.tar').then(async (r) => {
        const m = new Map()
        for (const [name, data] of untar(new Uint8Array(await r.arrayBuffer()))) {
          m.set(name, new File(data, { readonly: true }))
        }
        return m
      }),
    ])

    // Warm up: checks the toolchain works.
    const { module, stderr } = await compile('package main\n\nfunc main() {}\n')
    if (!module) throw new Error(stderr)
    self.postMessage({ type: 'ready' })
  } catch (e) {
    self.postMessage({ type: 'error', error: String(e) })
    throw e
  }
})()
