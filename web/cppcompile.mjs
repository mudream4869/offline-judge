// Compiles C++ to a WASI module with clang (YoWASP). Long-lived: loading
// clang is slow, so programs run in wasirun.mjs, which can be killed freely.
//
// in:  {id, code, grader?}  (grader: grader.cpp, linked with the code)
// out: {type: "ready"} | {type: "error", error}
//      {type: "result", id, status: "ok"|"ce", module, stderr, ms}

import { FLAGS, HEADER } from './cppflags.mjs'

const LINK = ['-Wl,-z,stack-size=67108864']

// The wasi libc++ is built without exceptions: make throw abort (RE).
// No #include, so it costs next to nothing to compile.
const EH = `
extern "C" {
void *malloc(__SIZE_TYPE__);
void abort(void);
long write(int, const void *, __SIZE_TYPE__);
void *__cxa_allocate_exception(__SIZE_TYPE__ n) { return malloc(n); }
void __cxa_throw(void *, void *, void (*)(void *)) {
  static const char msg[] = "terminate: uncaught exception (exceptions are not supported)\\n";
  write(2, msg, sizeof msg - 1);
  abort();
}
}
`

const USES_STDCXX = /#\s*include\s*<bits\/stdc\+\+\.h>/

let runClang = null
let header = ''
let pch = null // optional

async function compile(code, grader) {
  let stderr = ''
  const dec = new TextDecoder()
  const opts = { stderr: (b) => { if (b) stderr += dec.decode(b, { stream: true }) } }
  const bits = { 'stdc++.h': header }
  const args = ['clang++', ...FLAGS, '-Iinc']
  if (pch && (USES_STDCXX.test(code) || USES_STDCXX.test(grader ?? ''))) {
    bits['stdc++.h.pch'] = pch
    args.push('-include-pch', HEADER + '.pch', '-Xclang', '-fno-validate-pch')
  }
  const files = { 'eh.cpp': EH, inc: { bits } }
  if (grader) {
    files['solution.cpp'] = code
    files['grader.cpp'] = grader
    args.push(...LINK, 'grader.cpp', 'solution.cpp', 'eh.cpp', '-o', 'main.wasm')
  } else {
    files['main.cpp'] = code
    args.push(...LINK, 'main.cpp', 'eh.cpp', '-o', 'main.wasm')
  }
  try {
    const out = await runClang(args, files, opts)
    return { module: await WebAssembly.compile(out['main.wasm']), stderr }
  } catch (e) {
    // runClang throws Exit on a non-zero status.
    if (e.code === undefined) stderr += String(e)
    return { module: null, stderr }
  }
}

self.onmessage = async ({ data: { id, code, grader } }) => {
  await ready
  const t0 = performance.now()
  const { module, stderr } = await compile(code, grader)
  const ms = performance.now() - t0
  self.postMessage({ type: 'result', id, status: module ? 'ok' : 'ce', module, stderr, ms })
}

const ready = (async () => {
  try {
    // Imported here so a failure (e.g. offline) is reported, not a silent hang.
    runClang = (await import('./cpp/clang/bundle.js')).runClang
    const url = (name) => new URL(name, import.meta.url)
    header = await (await fetch(url('./stdc++.h'))).text()
    const resp = await fetch(url('./cpp/stdc++.h.pch'))
    if (resp.ok) pch = new Uint8Array(await resp.arrayBuffer())

    // Warm up: fetches and compiles clang, and checks the toolchain works.
    const { module, stderr } = await compile('#include <bits/stdc++.h>\nint main() {}')
    if (!module) throw new Error(stderr)
    self.postMessage({ type: 'ready' })
  } catch (e) {
    self.postMessage({ type: 'error', error: String(e) })
    throw e
  }
})()
