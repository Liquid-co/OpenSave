// REST + WebSocket client for the embedded OpenSave daemon.

let baseURL = '';

/** Resolve the daemon address from the Wails-bound Go method. */
export async function initApi() {
  // In `wails dev` / browser the binding may be absent — fall back to the
  // default daemon port.
  try {
    const info = await window.go.main.App.DaemonAddr();
    if (info.error) throw new Error(info.error);
    baseURL = `http://${info.addr}`;
  } catch (e) {
    if (e.message && !String(e.message).includes('undefined')) throw e;
    baseURL = 'http://127.0.0.1:8383';
  }
  return baseURL;
}

async function request(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(baseURL + path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || `${method} ${path} failed (${res.status})`);
  }
  return data;
}

export const api = {
  get: (path) => request('GET', path),
  post: (path, body) => request('POST', path, body ?? {}),
  patch: (path, body) => request('PATCH', path, body),
  del: (path) => request('DELETE', path)
};

/** Open the live-update WebSocket; onMessage receives {type, data}. */
export function connectWS(onMessage, onState) {
  let ws = null;
  let closed = false;

  const connect = () => {
    ws = new WebSocket(baseURL.replace('http', 'ws') + '/ws');
    ws.onopen = () => onState?.(true);
    ws.onmessage = (ev) => {
      try {
        onMessage(JSON.parse(ev.data));
      } catch {}
    };
    ws.onclose = () => {
      onState?.(false);
      if (!closed) setTimeout(connect, 2000);
    };
  };
  connect();

  return () => {
    closed = true;
    ws?.close();
  };
}

// ── Native bridges (no-ops outside Wails) ─────────────────────────────

const app = () => window.go?.main?.App;

export const native = {
  selectDirectory: (title) => app()?.SelectDirectory(title ?? '') ?? Promise.resolve(''),
  selectFile: (title) => app()?.SelectFile(title ?? '') ?? Promise.resolve(''),
  selectSaveFile: (title, name) => app()?.SelectSaveFile(title ?? '', name ?? '') ?? Promise.resolve(''),
  openExternal: (url) => app()?.OpenExternal(url),
  minimise: () => app()?.WindowMinimise(),
  toggleMaximise: () => app()?.WindowToggleMaximise(),
  close: () => app()?.WindowClose(),
  isWails: () => !!app()
};
