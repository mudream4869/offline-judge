// Starts checkbox.mjs in a data: URL worker and relays messages to it. A
// data: worker has an opaque origin, so the checker can't reach this site's
// IndexedDB, Cache Storage or OPFS whatever lockdown misses. The Go side
// kills this worker when a check takes too long, and the inner one with it.
//
// Requests from a data: worker skip the service worker, so the sources are
// fetched here (offline too) and inlined instead of imported.

const src = async name => (await fetch(new URL(name, import.meta.url))).text()

try {
  const box = (await src('sandbox.mjs')).replace(/^export /gm, '') +
    (await src('checkbox.mjs')).replace(/^import .*$/m, '')
  const inner = new Worker('data:text/javascript,' + encodeURIComponent(box), { type: 'module' })
  inner.onmessage = ({ data }) => self.postMessage(data)
  inner.onerror = e => self.postMessage({ type: 'error', error: `checker 環境載入失敗：${e.message}` })
  self.onmessage = ({ data }) => inner.postMessage(data)
} catch (e) {
  self.postMessage({ type: 'error', error: String(e) })
}
