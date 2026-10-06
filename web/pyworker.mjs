// Runs Python submissions with Pyodide. One worker = one Pyodide instance;
// the Go side terminates it on timeout and starts another.
//
// in:  {id, code, stdin}
// out: {type: "ready"} | {type: "error", error}
//      {type: "result", id, status: "ok"|"re", stdout, stderr, ms, fatal}

import { loadPyodide } from './pyodide/pyodide.mjs'

// Fresh stdio and globals per run, so runs don't leak into each other.
const HARNESS = `
import sys, io, time, traceback

def _judge_run(code, data):
    sin = io.TextIOWrapper(io.BytesIO(data.encode()), encoding="utf-8")
    out = io.BytesIO()
    sout = io.TextIOWrapper(out, encoding="utf-8", write_through=True)
    err = io.StringIO()
    saved = sys.stdin, sys.stdout, sys.stderr
    sys.stdin, sys.stdout, sys.stderr = sin, sout, err
    status = "ok"
    t0 = time.perf_counter()
    try:
        exec(compile(code, "main.py", "exec"), {"__name__": "__main__"})
    except SystemExit as e:
        if e.code not in (None, 0):
            status = "re"
            err.write(f"SystemExit: {e.code}\\n")
    except BaseException as e:
        status = "re"
        # Drop the harness frame from the traceback.
        traceback.print_exception(type(e), e, e.__traceback__.tb_next, file=err)
    finally:
        ms = (time.perf_counter() - t0) * 1000
        try:
            sout.flush()
        except Exception:
            pass
        sys.stdin, sys.stdout, sys.stderr = saved
    return status, out.getvalue().decode("utf-8", "replace"), err.getvalue(), ms
`

let run = null

self.onmessage = async ({ data: { id, code, stdin } }) => {
  await ready
  try {
    const res = run(code, stdin)
    const [status, stdout, stderr, ms] = res.toJs()
    res.destroy()
    self.postMessage({ type: 'result', id, status, stdout, stderr, ms, fatal: false })
  } catch (e) {
    // A JS-level error here (e.g. wasm stack overflow) leaves Pyodide unusable.
    self.postMessage({
      type: 'result', id, status: 're', stdout: '', stderr: String(e), ms: 0, fatal: true,
    })
  }
}

const ready = (async () => {
  try {
    const py = await loadPyodide({ indexURL: new URL('./pyodide/', import.meta.url).href })
    py.runPython(HARNESS)
    run = py.globals.get('_judge_run')
    self.postMessage({ type: 'ready' })
  } catch (e) {
    self.postMessage({ type: 'error', error: String(e) })
    throw e
  }
})()
