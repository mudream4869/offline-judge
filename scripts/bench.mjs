// Times each problem's reference solutions (<id>/_solutions/) in headless
// Chromium with the site's own workers, to help pick time limits.
//
//   scripts/build.sh                  # bench uses dist/
//   node scripts/bench.mjs [id...] [--runs N] [--cap MS]
//
// ac.* should be AC, tle.* (a too slow algorithm) should not. A run is
// killed at --cap ms (default 10000). With --runs, ac keeps each case's
// slowest time and tle its fastest.

import { createServer } from 'node:http'
import { execSync } from 'node:child_process'
import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { createRequire } from 'node:module'
import { extname, join, resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const ROOT = resolve(import.meta.dirname, '..')
const DIST = join(ROOT, 'dist')
const PROBLEMS = join(ROOT, 'problems')
const LANGS = { '.py': 'py', '.cpp': 'cpp', '.js': 'js', '.go': 'go' }
const NAMES = { py: 'Python', cpp: 'C++', js: 'JavaScript', go: 'Go' }

const args = process.argv.slice(2)
const opt = (name, def) => {
  const i = args.indexOf(name)
  return i < 0 ? def : Number(args.splice(i, 2)[1])
}
const runs = opt('--runs', 1)
const cap = opt('--cap', 10000)
const ids = args.length ? args : readdirSync(PROBLEMS)
  .filter((id) => existsSync(join(PROBLEMS, id, '_solutions'))).sort()

if (!existsSync(join(DIST, 'assets', 'pyworker.mjs'))) {
  console.error('dist/ not found: run scripts/build.sh first')
  process.exit(2)
}

// Playwright is usually global (npm i -g playwright).
async function loadPlaywright() {
  try {
    return await import('playwright')
  } catch {
    const root = execSync('npm root -g').toString().trim()
    return createRequire(join(root, 'noop.js'))('playwright')
  }
}

// ---- page: runs the workers the same way cmd/offline-judge does ----

const PAGE = `<!doctype html><script type="module">
const url = (n) => new URL('/assets/' + n, location.href).href

// One worker at a time; killed and respawned on timeout or a fatal result.
class Pool {
  constructor(name) { this.name = name; this.spawn() }
  spawn() {
    this.w = new Worker(url(this.name), { type: 'module' })
    this.waiters = new Map()
    this.ready = new Promise((ok, fail) => {
      this.w.onmessage = ({ data }) => {
        if (data.type === 'ready') ok()
        else if (data.type === 'error') fail(new Error(data.error))
        else this.waiters.get(data.id)?.(data)
      }
    })
  }
  async call(msg, timeout) {
    await this.ready
    const id = (this.seq = (this.seq || 0) + 1)
    const res = await new Promise((ok) => {
      const t = setTimeout(() => ok(null), timeout)
      this.waiters.set(id, (d) => { clearTimeout(t); ok(d) })
      this.w.postMessage({ ...msg, id })
    })
    if (!res || res.fatal) { this.w.terminate(); this.spawn() }
    return res
  }
}

const pools = {}
const pool = (name) => (pools[name] ||= new Pool(name))
const COMPILER = { cpp: 'cppcompile.mjs', go: 'gocompile.mjs' }
const RUNNER = { py: 'pyworker.mjs', js: 'jsrun.mjs' }
const modules = []

// compile returns {key} or {ce}; interpreted languages keep the source.
window.compile = async (lang, code) => {
  if (!COMPILER[lang]) return { key: modules.push(code) - 1 }
  const r = await pool(COMPILER[lang]).call({ code }, 120000)
  if (!r) return { ce: 'compile timeout' }
  if (r.status !== 'ok') return { ce: r.stderr }
  return { key: modules.push(r.module) - 1 }
}

// run returns the worker's result, or {status: 'tle'} after cap ms.
window.run = async (lang, key, stdin, interactor, cap) => {
  const msg = { stdin, ...(interactor ? { interactor } : {}) }
  if (COMPILER[lang]) msg.module = modules[key]
  else msg.code = modules[key]
  const r = await pool(RUNNER[lang] || 'wasirun.mjs').call(msg, cap)
  if (!r) return { status: 'tle', ms: cap }
  const { module, ...rest } = r
  return rest
}
window.loaded = true
</script>`

const MIME = {
  '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript',
  '.wasm': 'application/wasm', '.json': 'application/json',
}

function serve() {
  const srv = createServer((req, res) => {
    const path = decodeURIComponent(new URL(req.url, 'http://x').pathname)
    if (path === '/bench.html') {
      res.writeHead(200, { 'content-type': 'text/html' }).end(PAGE)
      return
    }
    const file = join(DIST, path)
    if (!file.startsWith(DIST) || !existsSync(file)) {
      res.writeHead(404).end()
      return
    }
    res.writeHead(200, { 'content-type': MIME[extname(file)] || 'application/octet-stream' })
    res.end(readFileSync(file))
  })
  return new Promise((ok) => srv.listen(0, '127.0.0.1', () => ok(srv)))
}

// ---- problems ----

// normalize matches internal/judge.
const normalize = (s) => s.replace(/\r\n/g, '\n').split('\n')
  .map((l) => l.replace(/[ \t\r]+$/, '')).join('\n').replace(/\n+$/, '')

async function load(id) {
  const dir = join(PROBLEMS, id)
  const meta = JSON.parse(readFileSync(join(dir, 'problem.json'), 'utf8'))
  const cases = readdirSync(join(dir, 'tests')).filter((f) => f.endsWith('.in')).sort()
    .map((f) => {
      const name = f.slice(0, -3)
      const out = join(dir, 'tests', name + '.out')
      return {
        name,
        input: readFileSync(join(dir, 'tests', f), 'utf8'),
        output: existsSync(out) ? readFileSync(out, 'utf8') : '',
      }
    })
  const read = (f) => (existsSync(join(dir, f)) ? readFileSync(join(dir, f), 'utf8') : '')
  const checker = read('checker.js')
    ? (await import(pathToFileURL(join(dir, 'checker.js')))).default : null
  const solutions = readdirSync(join(dir, '_solutions')).filter((f) => LANGS[extname(f)]).sort()
  return { id, dir, meta, cases, checker, interactor: read('interactor.js'), solutions }
}

const limitOf = (meta, lang) => meta.time_limits_ms?.[lang] ?? meta.time_limit_ms

// verdict judges one result like cmd/offline-judge, ignoring the time limit.
async function verdict(p, c, r) {
  if (r.status === 'tle') return 'TLE'
  if (r.iaError) throw new Error(r.iaError)
  if (p.interactor) return r.judged?.ok ? 'AC' : 'WA'
  if (r.status === 're') return 'RE'
  if (p.checker) return (await p.checker(c.input, r.stdout, c.output)) === true ? 'AC' : 'WA'
  return normalize(r.stdout) === normalize(c.output) ? 'AC' : 'WA'
}

// bench returns the solution's verdict per case and its slowest case.
async function bench(page, p, file) {
  const lang = LANGS[extname(file)]
  const code = readFileSync(join(p.dir, '_solutions', file), 'utf8')
  const expectAC = file.startsWith('ac')
  const { key, ce } = await page.evaluate(([l, c]) => compile(l, c), [lang, code])
  if (ce !== undefined) return { lang, expectAC, verdict: 'CE', detail: ce.split('\n')[0] }

  let worst = null
  for (const c of p.cases) {
    let best = null
    for (let i = 0; i < runs; i++) {
      const r = await page.evaluate((a) => run(...a), [lang, key, c.input, p.interactor, cap])
      // ac: slowest run; tle: fastest.
      if (!best || (expectAC ? r.ms > best.ms : r.ms < best.ms)) best = r
      if (r.status === 'tle') break
    }
    const v = await verdict(p, c, best)
    if (v !== 'AC' && v !== 'TLE') return { lang, expectAC, verdict: v, case: c.name, ms: best.ms }
    if (!worst || best.ms > worst.ms) worst = { case: c.name, ms: best.ms, tle: v === 'TLE' }
    // Like the judge: give up after a kill.
    if (v === 'TLE') break
  }
  return { lang, expectAC, verdict: 'ok', ...worst }
}

// ---- report ----

const fmtMs = (r) => (r.tle ? `>${cap}` : r.ms.toFixed(0)).padStart(7) + ' ms'

let failed = false
const pw = await loadPlaywright()
const srv = await serve()
const browser = await pw.chromium.launch()
try {
  const page = await browser.newPage()
  page.on('pageerror', (e) => console.error('page error:', e))
  await page.goto(`http://127.0.0.1:${srv.address().port}/bench.html`)
  await page.waitForFunction(() => window.loaded)

  for (const id of ids) {
    const p = await load(id)
    console.log(`\n${id}  ${p.meta.title}`)
    const byLang = {}
    for (const file of p.solutions) {
      const lang = LANGS[extname(file)]
      if (p.interactor && lang === 'js') {
        console.log(`  ${file.padEnd(10)} 略過（JavaScript 尚未支援互動題）`)
        continue
      }
      const r = await bench(page, p, file)
      const limit = limitOf(p.meta, lang)
      let mark
      if (r.verdict !== 'ok') {
        mark = `✗ ${r.verdict}${r.case ? ' @ ' + r.case : ''}${r.detail ? ': ' + r.detail : ''}`
        failed = true
      } else {
        const pass = !r.tle && r.ms <= limit
        mark = `${(r.ms / limit).toFixed(2).padStart(6)}× 時限 @ ${r.case}`
        if (pass !== r.expectAC) {
          mark += r.expectAC ? '  ✗ 超時' : '  ✗ 會通過'
          failed = true
        } else if (r.expectAC ? r.ms * 2 > limit : r.ms < limit * 2) {
          mark += '  ! 差距不到 2 倍'
        }
        ;(byLang[lang] ||= { limit })[r.expectAC ? 'ac' : 'tle'] = r
      }
      console.log(`  ${file.padEnd(10)} ${r.ms === undefined ? ' '.repeat(10) : fmtMs(r)} ${mark}`)
    }
    for (const [lang, { limit, ac, tle }] of Object.entries(byLang)) {
      if (!ac) continue
      const lo = Math.ceil(ac.ms * 2 / 100) * 100
      const hi = !tle ? '' : tle.tle ? `（tle 超過 ${cap} ms）` : `，且遠低於 ${Math.floor(tle.ms)} ms（tle）`
      console.log(`  ${NAMES[lang].padEnd(10)} 目前 ${limit} ms；建議 ≥ ${lo} ms${hi}`)
    }
  }
} finally {
  await browser.close()
  srv.close()
}
process.exit(failed ? 1 : 0)
