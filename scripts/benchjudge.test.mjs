import assert from 'node:assert/strict'
import { after, test } from 'node:test'

import { benchCases, createJudge } from './benchjudge.mjs'

const judge = createJudge()
after(() => judge.close())
const problem = (compare = '') => ({ meta: { compare }, interactor: '', checker: null })
const tc = { input: 'input', output: '1\n' }
const run = (stdout = '1\n', extra = {}) => ({ status: 'ok', stdout, stderr: '', ms: 1, ...extra })

for (const [name, compare, output, answer, expected] of [
  ['default trailing whitespace', '', '1 \r\n \t\n', '1\n', 'AC'],
  ['default internal whitespace', '', '1  2\n', '1 2\n', 'WA'],
  ['strict trailing space', 'strict', '1 \n', '1\n', 'WA'],
  ['strict missing newline', 'strict', '1', '1\n', 'WA'],
  ['strict CRLF', 'strict', '1\r\n', '1\n', 'WA'],
  ['strict exact output', 'strict', '1\n', '1\n', 'AC'],
  ['white-diff whitespace', 'white-diff', ' 1\t  2\n', '1 2\n', 'AC'],
  ['white-diff Unicode whitespace', 'white-diff', '1\u00852\n', '1 2\n', 'AC'],
  ['white-diff preserves lines', 'white-diff', '1 2\n', '1\n2\n', 'WA'],
  ['float default tolerance', 'float-diff', '0.100001\n', '0.1\n', 'AC'],
  ['float outside tolerance', 'float-diff', '0.10001\n', '0.1\n', 'WA'],
  ['float integers stay exact', 'float-diff', '2.0\n', '2\n', 'WA'],
  ['float absolute tolerance', 'float-diff absolute 0.5', '1.4\n', '1.0\n', 'AC'],
  ['float absolute rejects large error', 'float-diff absolute 1e-6', '1000001.5\n', '1000000.5\n', 'WA'],
  ['float relative tolerance', 'float-diff relative 1e-6', '1000001.5\n', '1000000.5\n', 'AC'],
  ['float relative zero', 'float-diff relative 1e-6', '1e-7\n', '0.0\n', 'WA'],
  ['float either tolerance', 'float-diff absolute-relative 1e-6', '1e-7\n', '0.0\n', 'AC'],
]) {
  test(name, async () => {
    assert.equal(await judge.verdict(problem(compare), { ...tc, output: answer }, run(output)), expected)
  })
}

for (const [name, status, judged, expected] of [
  ['interactive accepted', 'ok', { ok: true }, 'AC'],
  ['interactive rejected', 'ok', { ok: false, message: 'wrong guess' }, 'WA'],
  ['interactive rejection before RE', 're', { ok: false }, 'WA'],
  ['interactive acceptance cannot hide RE', 're', { ok: true }, 'RE'],
  ['interactive RE without judgement', 're', null, 'RE'],
  ['timeout before interactive rejection', 'tle', { ok: false }, 'TLE'],
  ['timeout before acceptance', 'tle', { ok: true }, 'TLE'],
]) {
  test(name, async () => {
    assert.equal(await judge.verdict({ ...problem(), interactor: 'interactor.js' }, tc, run('', { status, judged })), expected)
  })
}

test('interactive missing judgement is an infrastructure error', async () => {
  await assert.rejects(judge.verdict({ ...problem(), interactor: 'interactor.js' }, tc, run()), /互動程式沒有回報結果/)
})

test('interactor exception is an infrastructure error', async () => {
  await assert.rejects(judge.verdict({ ...problem(), interactor: 'interactor.js' }, tc, run('', { iaError: 'broken' })), /互動程式錯誤：broken/)
})

for (const [name, value, expected] of [
  ['checker true', true, 'AC'],
  ['checker false', false, 'WA'],
  ['checker message', 'wrong answer', 'WA'],
  ['checker empty message', '', 'WA'],
]) {
  test(name, async () => {
    const p = { ...problem(), checker: (input, output, answer) => {
      assert.equal(input, tc.input)
      assert.equal(output, 'different valid answer')
      assert.equal(answer, tc.output)
      return Promise.resolve(value)
    } }
    assert.equal(await judge.verdict(p, tc, run('different valid answer')), expected)
  })
}

for (const value of [undefined, null, 1, {}, []]) {
  test(`invalid checker result: ${JSON.stringify(value)}`, async () => {
    await assert.rejects(judge.verdict({ ...problem(), checker: () => value }, tc, run()), /checker 應回傳/)
  })
}

test('checker exception is an infrastructure error', async () => {
  await assert.rejects(judge.verdict({ ...problem(), checker: () => { throw new Error('broken checker') } }, tc, run()), /broken checker/)
})

for (const status of ['re', 'tle', 'ce']) {
  test(`checker skipped after ${status}`, async () => {
    const p = { ...problem(), checker: () => assert.fail('checker should not run') }
    assert.equal(await judge.verdict(p, tc, run('', { status })), { re: 'RE', tle: 'TLE', ce: 'CE' }[status])
  })
}

test('invalid compare is rejected', async () => {
  await assert.rejects(judge.verdict(problem('float-diff -1'), tc, run()), /誤差有誤/)
})

test('recorded duration does not impose the problem limit during calibration', async () => {
  const p = { ...problem(), meta: { time_limit_ms: 1 } }
  assert.equal(await judge.verdict(p, tc, run('1\n', { ms: 100000 })), 'AC')
})

for (const [name, expectAC, results, expected] of [
  ['RE cannot hide behind a slower successful run', true, [run('1\n', { ms: 100 }), run('', { status: 're', ms: 1 })], { verdict: 'RE', case: '01', ms: 1 }],
  ['WA cannot hide behind a faster successful run', false, [run('1\n', { ms: 1 }), run('wrong', { ms: 100 })], { verdict: 'WA', case: '01', ms: 100 }],
  ['ac uses slowest valid run', true, [run('1\n', { ms: 10 }), run('1\n', { ms: 5 })], { verdict: 'ok', case: '01', ms: 10, tle: false }],
  ['tle uses fastest valid run', false, [run('1\n', { ms: 10 }), run('1\n', { ms: 5 })], { verdict: 'ok', case: '01', ms: 5, tle: false }],
]) {
  test(name, async () => {
    let i = 0
    const result = await benchCases([{ ...tc, name: '01' }], () => results[i++],
      (c, r) => judge.verdict(problem(), c, r), { runs: 2, expectAC })
    assert.deepEqual(result, expected)
    assert.equal(i, 2)
  })
}

test('a killed case skips remaining runs and cases', async () => {
  let calls = 0
  const result = await benchCases([{ ...tc, name: '01' }, { ...tc, name: '02' }], () => {
    calls++
    return run('', { status: 'tle', ms: 100 })
  }, (c, r) => judge.verdict(problem(), c, r), { runs: 2, expectAC: true })
  assert.deepEqual(result, { verdict: 'ok', case: '01', ms: 100, tle: true })
  assert.equal(calls, 1)
})

test('an interactor error in any repeat fails the benchmark', async () => {
  let calls = 0
  const p = { ...problem(), interactor: 'interactor.js' }
  await assert.rejects(benchCases([{ ...tc, name: '01' }], () => {
    calls++
    return run('', calls === 1 ? { judged: { ok: true }, ms: 100 } : { iaError: 'broken', ms: 1 })
  }, (c, r) => judge.verdict(p, c, r), { runs: 2, expectAC: true }), /互動程式錯誤：broken/)
})

test('a timeout cannot hide behind an equal successful duration', async () => {
  let calls = 0
  const result = await benchCases([{ ...tc, name: '01' }], () =>
    ++calls === 1 ? run('1\n', { ms: 100 }) : run('', { status: 'tle', ms: 100 }),
  (c, r) => judge.verdict(problem(), c, r), { runs: 2, expectAC: true })
  assert.deepEqual(result, { verdict: 'ok', case: '01', ms: 100, tle: true })
})

test('a timeout case cannot hide behind an earlier equal duration', async () => {
  const result = await benchCases([{ ...tc, name: '01' }, { ...tc, name: '02' }],
    (c) => run(c.name === '01' ? '1\n' : '', { ms: 100, status: c.name === '01' ? 'ok' : 'tle' }),
    (c, r) => judge.verdict(problem(), c, r), { runs: 1, expectAC: true })
  assert.deepEqual(result, { verdict: 'ok', case: '02', ms: 100, tle: true })
})

test('tle calibration keeps the fastest completed repeat before a later timeout', async () => {
  let calls = 0
  const result = await benchCases([{ ...tc, name: '01' }], () =>
    ++calls === 1 ? run('1\n', { ms: 5 }) : run('', { status: 'tle', ms: 100 }),
  (c, r) => judge.verdict(problem(), c, r), { runs: 2, expectAC: false })
  assert.deepEqual(result, { verdict: 'ok', case: '01', ms: 5, tle: false })
})
