// Run the js/wasm Go tests against real IndexedDB in an isolated Chromium worker.
// Uses the same Playwright installation as scripts/bench.mjs.
import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { createServer } from 'node:http'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const dir = mkdtempSync(join(tmpdir(), 'offline-judge-browser-tests-'))
let server, browser
try {
  const wasm = join(dir, 'tests.wasm')
  execFileSync('go', ['test', '-c', '-o', wasm, './cmd/offline-judge'], {
    cwd: root, env: { ...process.env, GOOS: 'js', GOARCH: 'wasm' }, stdio: 'inherit',
  })
  const goroot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8' }).trim()
  let pw
  try {
    pw = await import('playwright')
  } catch {
    const modules = execFileSync('npm', ['root', '-g'], { encoding: 'utf8' }).trim()
    pw = createRequire(join(modules, 'noop.js'))('playwright')
  }
  const assets = {
    '/': ['text/html; charset=utf-8', `<!doctype html><meta charset="utf-8"><script>
      const worker = new Worker('/runner.js')
      worker.onmessage = ({data}) => window.result = data
      worker.onerror = e => window.result = {error: e.message}
    </script>`],
    '/runner.js': ['text/javascript', `
      importScripts('/wasm_exec.js')
      const go = new Go()
      let code = 0
      go.argv = ['storage-tests', '-test.v', '-test.timeout=30s']
      go.exit = value => code = value
      WebAssembly.instantiateStreaming(fetch('/tests.wasm'), go.importObject)
        .then(({instance}) => go.run(instance))
        .then(() => postMessage({code}))
        .catch(e => postMessage({error: String(e.stack || e)}))
    `],
    '/wasm_exec.js': ['text/javascript', readFileSync(join(goroot, 'lib', 'wasm', 'wasm_exec.js'))],
    '/tests.wasm': ['application/wasm', readFileSync(wasm)],
  }
  server = createServer((req, res) => {
    const asset = assets[req.url]
    if (!asset) { res.writeHead(404).end(); return }
    res.writeHead(200, { 'content-type': asset[0] }).end(asset[1])
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  browser = await pw.chromium.launch()
  const page = await browser.newPage()
  page.on('console', message => console.log(message.text()))
  await page.goto(`http://127.0.0.1:${server.address().port}/`)
  await page.waitForFunction(() => window.result !== undefined, null, { timeout: 45000 })
  const result = await page.evaluate(() => window.result)
  if (result.error) throw new Error(result.error)
  process.exitCode = result.code
} finally {
  await browser?.close()
  if (server) await new Promise(resolve => server.close(resolve))
  rmSync(dir, { recursive: true, force: true })
}
