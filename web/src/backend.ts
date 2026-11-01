// Backend transport. In the desktop app, calls go through the Wails runtime;
// in a browser (`<app> serve`, or Vite in development), through HTTP and
// server-sent events (the Go package rpc). Both reach the same Go service.

export const isWails =
  typeof window !== 'undefined' && (window.location.protocol === 'wails:' || window.location.hostname === 'wails.localhost');

export interface BackendOptions {
  /** The Go service's Wails name: its package path and type, "github.com/jespino/kino/internal/app.App". */
  service: string;
  /** What to tell the user when no backend answers (a browser without the server). */
  noBackend?: string;
  /** Names the BroadcastChannel that carries `broadcast` events between browser tabs. */
  channel?: string;
}

export interface Backend {
  isWails: boolean;
  /** Calls a method of the Go service. */
  call<T>(method: string, ...args: unknown[]): Promise<T>;
  /** Listens to an event; returns the function that stops listening. */
  on<T>(name: string, fn: (data: T) => void): () => void;
  /** Sends an event to the app's other windows (or browser tabs). */
  broadcast(name: string, data: unknown): Promise<void>;
}

/** The text of an error, without the "Error: " prefix. */
export const errorText = (e: unknown) => String(e instanceof Error ? e.message : e).replace(/^Error: /, '');

type Listener = (data: unknown) => void;

export function createBackend(options: BackendOptions): Backend {
  const noBackend = options.noBackend ?? 'No backend is reachable. In a browser, start the app with `serve`; the desktop app has it built in.';
  const listeners = new Map<string, Set<Listener>>();
  const dispatch = (name: string, data: unknown) => listeners.get(name)?.forEach((fn) => fn(data));

  let wails: typeof import('@wailsio/runtime') | null = null;
  const ready: Promise<void> = (async () => {
    if (isWails) {
      wails = await import('@wailsio/runtime');
      return;
    }
    if (typeof EventSource === 'undefined') return;
    const es = new EventSource('/events');
    es.onmessage = (ev) => {
      const msg = JSON.parse(ev.data) as { name: string; data: unknown };
      dispatch(msg.name, msg.data);
    };
  })();

  const tabs = !isWails && typeof BroadcastChannel !== 'undefined' && options.channel ? new BroadcastChannel(options.channel) : null;
  if (tabs) tabs.onmessage = (ev: MessageEvent<{ name: string; data: unknown }>) => dispatch(ev.data.name, ev.data.data);

  async function call<T>(method: string, ...args: unknown[]): Promise<T> {
    await ready;
    if (wails) return (await wails.Call.ByName(`${options.service}.${method}`, ...args)) as T;
    let res: Response;
    try {
      res = await fetch(`/rpc/${method}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(args) });
    } catch {
      throw new Error(noBackend);
    }
    // a dev proxy with nothing behind it answers with an HTML/text error page
    if (!(res.headers.get('content-type') ?? '').includes('application/json')) {
      if (res.status === 403) throw new Error(`${(await res.text()).trim() || 'Forbidden'} (the backend refused the request)`);
      throw new Error(noBackend);
    }
    const body = (await res.json()) as { result?: T; error?: string };
    if (body.error) throw new Error(body.error);
    return body.result as T;
  }

  function on<T>(name: string, fn: (data: T) => void): () => void {
    let set = listeners.get(name);
    if (!set) listeners.set(name, (set = new Set()));
    set.add(fn as Listener);
    let off: (() => void) | undefined;
    let gone = false;
    if (isWails) {
      ready.then(() => {
        if (!gone) off = wails!.Events.On(name, (ev: { data: unknown }) => fn(ev.data as T));
      });
    }
    return () => {
      gone = true;
      set!.delete(fn as Listener);
      off?.();
    };
  }

  async function broadcast(name: string, data: unknown) {
    if (isWails) {
      await ready;
      await wails!.Events.Emit(name, data);
    } else tabs?.postMessage({ name, data });
  }

  return { isWails, call, on, broadcast };
}
