// Runs a problem's checker.js, inside the opaque-origin worker checker.mjs
// starts. Not loaded as a file: checker.mjs inlines it with sandbox.mjs.
//
// checker.js: export default function check(input, output, answer)
//   returns true (AC), false or a message string (WA)
//
// in:  {id, checker, input, output, answer}
// out: {type: "ready"}
//      {type: "result", id, ok, message, error}

import { lockdown, importDefault, verdict } from './sandbox.mjs'

lockdown()

self.onmessage = async ({ data: { id, checker, input, output, answer } }) => {
  try {
    const check = await importDefault(checker, 'checker.js')
    const { ok, message } = verdict(await check(input, output, answer), 'checker')
    self.postMessage({ type: 'result', id, ok, message })
  } catch (e) {
    self.postMessage({ type: 'result', id, ok: false, message: '', error: String(e?.stack || e) })
  }
}

self.postMessage({ type: 'ready' })
