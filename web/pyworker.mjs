// Runs Python submissions with Pyodide. One worker = one Pyodide instance;
// the Go side terminates it on timeout and starts another.
//
// in:  {id, code, stdin, interactor?, outputLimit?}
// out: {type: "ready"} | {type: "error", error}
//      {type: "result", id, status: "ok"|"re"|"ole", stdout, stderr, ms, fatal,
//       judged?, iaError?}
//
// With interactor (interactor.js source), stdin is its input and the
// program reads what it answers; stdout is the transcript (sandbox.mjs).

import { loadPyodide } from './pyodide/pyodide.mjs'
import { lockdown, importDefault, Interaction, interactionResult } from './sandbox.mjs'

// Fresh stdio and globals per run, so runs don't leak into each other.
const HARNESS = `
import sys, io, time, traceback

class _Interactive(io.RawIOBase):
    """Reads what ask answers, given the output since the last read."""

    def __init__(self, ask, out):
        self.ask, self.out, self.sent, self.pending = ask, out, 0, b""

    def readable(self):
        return True

    def readinto(self, b):
        if not self.pending:
            new = self.out.getbuffer()[self.sent:].tobytes()
            self.sent += len(new)
            r = self.ask(new.decode("utf-8", "replace"))
            if r is None:
                return 0
            self.pending = r.encode()
        n = min(len(b), len(self.pending))
        b[:n] = self.pending[:n]
        self.pending = self.pending[n:]
        return n

class _OutputLimit(BaseException):
    pass

class _Out(io.BytesIO):
    """Stdout that stops the program once it passes limit bytes."""

    def __init__(self, limit):
        super().__init__()
        self.limit, self.over = limit, False

    def write(self, b):
        if self.over or self.tell() + len(b) > self.limit:
            self.over = True
            raise _OutputLimit
        return super().write(b)

def _judge_run(code, data, limit, ask=None):
    out = _Out(limit)
    raw = _Interactive(ask, out) if ask else None
    if raw:
        sin = io.TextIOWrapper(io.BufferedReader(raw), encoding="utf-8")
    else:
        sin = io.TextIOWrapper(io.BytesIO(data.encode()), encoding="utf-8")
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
        if not out.over:
            status = "re"
            # Drop the harness frame from the traceback.
            traceback.print_exception(type(e), e, e.__traceback__.tb_next, file=err)
    finally:
        ms = (time.perf_counter() - t0) * 1000
        try:
            sout.flush()
        except BaseException:
            pass
        sys.stdin, sys.stdout, sys.stderr = saved
    # Even if the program caught _OutputLimit.
    if out.over:
        status = "ole"
    rest = out.getvalue()[raw.sent if raw else 0:]
    return status, out.getvalue().decode("utf-8", "replace"), err.getvalue(), ms, \
        rest.decode("utf-8", "replace")
`

let run = null

self.onmessage = async ({ data: { id, code, stdin, interactor, outputLimit = Infinity } }) => {
  await ready
  let ia = null
  if (interactor) {
    // Pyodide has loaded everything it needs by now.
    lockdown()
    try {
      ia = new Interaction(await importDefault(interactor, 'interactor.js'), stdin)
    } catch (e) {
      self.postMessage({ type: 'result', id, status: 'ok', stdout: '', stderr: '', ms: 0,
        fatal: false, iaError: String(e?.stack || e) })
      return
    }
  }
  try {
    const res = ia
      ? run(code, '', outputLimit, (out) => ia.read(out) ?? undefined)
      : run(code, stdin, outputLimit)
    let [status, stdout, stderr, ms, rest] = res.toJs()
    res.destroy()
    let extra = {}
    if (ia) {
      extra = interactionResult(ia, rest)
      ms -= ia.ms
    }
    self.postMessage({ type: 'result', id, status, stdout, stderr, ms, fatal: false, ...extra })
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
