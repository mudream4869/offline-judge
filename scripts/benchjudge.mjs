// Build the native judge once per benchmark. Each case uses judge.Judge,
// keeping compare modes and verdict precedence in the site's Go package.
import { execFileSync } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

export function createJudge(root = resolve(import.meta.dirname, '..')) {
  const dir = mkdtempSync(join(tmpdir(), 'offline-judge-bench-'))
  const bin = join(dir, process.platform === 'win32' ? 'benchjudge.exe' : 'benchjudge')
  const close = () => rmSync(dir, { recursive: true, force: true })
  try {
    execFileSync('go', ['build', '-o', bin, './cmd/benchjudge'], { cwd: root, stdio: 'inherit' })
  } catch (e) {
    close()
    throw e
  }

  return {
    close,
    async verdict(p, c, result) {
      const req = {
        compare: p.meta.compare ?? '',
        interactive: Boolean(p.interactor),
        answer: c.output,
        result,
      }
      // Like the site's judge: check output only after a successful run.
      if (p.checker && !p.interactor && result.status === 'ok' && !result.iaError && result.judged?.ok !== false) {
        req.checker = { value: await p.checker(c.input, result.stdout, c.output) }
      }
      const res = JSON.parse(execFileSync(bin, {
        input: JSON.stringify(req),
        encoding: 'utf8',
      }))
      if (res.error) throw new Error(res.error)
      return res.verdict
    },
  }
}

// Validate every run before choosing the timing used for calibration.
export async function benchCases(cases, run, verdict, { runs, expectAC }) {
  let worst = null
  for (const c of cases) {
    let best = null
    for (let i = 0; i < runs; i++) {
      const r = await run(c)
      const v = await verdict(c, r)
      if (v !== 'AC' && v !== 'TLE') return { verdict: v, case: c.name, ms: r.ms }
      if (!best || (expectAC && v === 'TLE') || (expectAC ? r.ms > best.ms : r.ms < best.ms)) best = { ...r, verdict: v }
      if (v === 'TLE') break
    }
    const tle = best.verdict === 'TLE'
    if (tle || !worst || best.ms > worst.ms) worst = { case: c.name, ms: best.ms, tle }
    if (tle) break
  }
  return { verdict: 'ok', ...worst }
}
