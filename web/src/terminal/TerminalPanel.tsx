import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { EVENT_TERMINAL_DATA, EVENT_TERMINAL_EXIT, EVENT_TERMINAL_START, type Harness } from '../api';
import { errorText } from '../backend';
import { storage, studioConfig } from '../config';
import { agentInput, setActiveAgent, setTerminalSink } from './bus';

function decode(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function cssVar(name: string, fallback = '') {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}

export interface TerminalPanelProps {
  /** The panel's title. */
  title?: string;
  /** Shown before an agent starts: what the agent gets and does. */
  intro?: ReactNode;
}

/**
 * The agent panel: an embedded terminal running an agent harness (Claude
 * Code, Codex, Gemini CLI, …) in the project folder. The agent edits the
 * project; the studio picks the changes up live.
 *
 * Theme: --term-bg, --term-fg, --term-selection and --accent (the cursor).
 */
export function TerminalPanel({ title = 'Agent', intro }: TerminalPanelProps) {
  const { api, backend } = studioConfig();
  const host = useRef<HTMLDivElement>(null);
  const term = useRef<Terminal | null>(null);
  const fit = useRef<FitAddon | null>(null);
  const idRef = useRef<string | null>(null);
  const exited = useRef(false);
  const startingRef = useRef(false);
  const [id, setId] = useState<string | null>(null);
  const [harnesses, setHarnesses] = useState<Harness[]>([]);
  const [harness, setHarness] = useState(() => storage.get('harness', 'claude'));
  const [exitCode, setExitCode] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);

  // Keystrokes are sent strictly in order, batched while a write is in flight.
  const queue = useRef('');
  const flushing = useRef(false);
  const send = (data: string) => {
    queue.current += data;
    if (flushing.current || !idRef.current) return;
    flushing.current = true;
    (async () => {
      while (queue.current && idRef.current) {
        const chunk = queue.current;
        queue.current = '';
        await api.terminalWrite(idRef.current, chunk).catch(() => {});
      }
      flushing.current = false;
    })();
  };

  const attach = (newId: string | null, activeHarness?: string) => {
    idRef.current = newId;
    exited.current = false;
    setId(newId);
    setExitCode(null);
    setActiveAgent(
      newId && activeHarness !== 'shell'
        ? {
            send: (prompt) => api.terminalWrite(newId, agentInput(prompt)),
            focus: () => term.current?.focus(),
          }
        : null,
    );
  };

  useEffect(() => {
    let disposed = false;
    const t = new Terminal({
      fontFamily: 'ui-monospace, "JetBrains Mono", "Fira Code", monospace',
      fontSize: 13,
      cursorBlink: true,
      allowProposedApi: true,
      scrollback: 10000,
      theme: {
        background: cssVar('--term-bg', '#1b1e23'),
        foreground: cssVar('--term-fg', '#e6e8eb'),
        cursor: cssVar('--accent', '#3b74d9'),
        selectionBackground: cssVar('--term-selection', 'rgba(110,168,254,0.35)'),
      },
    });
    const f = new FitAddon();
    t.loadAddon(f);
    t.open(host.current!);
    term.current = t;
    fit.current = f;
    t.onData(send);
    t.onBinary(send);

    const ro = new ResizeObserver(() => {
      try {
        f.fit();
      } catch {
        return;
      }
      if (idRef.current) api.terminalResize(idRef.current, t.cols, t.rows).catch(() => {});
    });
    ro.observe(host.current!);

    const offData = backend.on<{ id: string; data: string }>(EVENT_TERMINAL_DATA, (ev) => {
      if (ev.id === idRef.current) t.write(decode(ev.data));
    });
    const offExit = backend.on<{ id: string; code: number }>(EVENT_TERMINAL_EXIT, (ev) => {
      if (ev.id === idRef.current) {
        exited.current = true;
        setActiveAgent(null);
        setExitCode(ev.code);
        t.write(`\r\n\x1b[2m[process exited with code ${ev.code}]\x1b[0m\r\n`);
      }
    });
    // An agent started elsewhere (another window, the API): show it here
    // unless this panel has one of its own.
    const offStart = backend.on<{ id: string; harness: string }>(EVENT_TERMINAL_START, (ev) => {
      if (startingRef.current || ev.id === idRef.current || (idRef.current && !exited.current)) return;
      t.reset();
      setHarness(ev.harness);
      attach(ev.id, ev.harness);
      try {
        f.fit();
      } catch {
        return;
      }
      api.terminalResize(ev.id, t.cols, t.rows).catch(() => {});
    });

    setTerminalSink((text) => {
      if (!idRef.current || exited.current) return false;
      send(text);
      t.focus();
      return true;
    });

    api.harnesses().then(setHarnesses, (e) => setError(errorText(e)));
    // Reattach to a session that survived a reload.
    api.terminals().then(
      async (list) => {
        if (disposed || idRef.current) return;
        const running = (list ?? []).find((x) => x.running);
        if (!running) return;
        setHarness(running.harness);
        const buf = await api.terminalBuffer(running.id);
        if (disposed || idRef.current) return;
        t.write(decode(buf));
        attach(running.id, running.harness);
        f.fit();
        api.terminalResize(running.id, t.cols, t.rows).catch(() => {});
      },
      () => {},
    );

    return () => {
      disposed = true;
      ro.disconnect();
      offData();
      offExit();
      offStart();
      setTerminalSink(null);
      setActiveAgent(null);
      t.dispose();
    };
  }, []);

  const start = async (h = harness, resume = false) => {
    if (startingRef.current) return;
    startingRef.current = true;
    setStarting(true);
    const t = term.current!;
    setError(null);
    storage.set('harness', h);
    if (idRef.current) await api.stopTerminal(idRef.current).catch(() => {});
    attach(null);
    t.reset();
    try {
      fit.current!.fit();
      const launch = resume ? api.resumeTerminal : api.startTerminal;
      attach(await launch(h, t.cols, t.rows), h);
      t.focus();
    } catch (e) {
      setError(errorText(e));
    } finally {
      startingRef.current = false;
      setStarting(false);
    }
  };

  const stop = async () => {
    if (idRef.current) await api.stopTerminal(idRef.current).catch(() => {});
    attach(null);
  };

  const current = harnesses.find((h) => h.id === harness);
  const running = id !== null && exitCode === null;

  return (
    <div className="panel terminal">
      <div className="panel-header">
        <span className="panel-title">{title}</span>
        <select value={harness} onChange={(e) => setHarness(e.target.value)} title="Agent harness (Restart runs the one chosen)" disabled={starting}>
          {harnesses.map((h) => (
            <option key={h.id} value={h.id} disabled={!h.available}>
              {h.name}
              {h.available ? '' : ' (not installed)'}
            </option>
          ))}
        </select>
        <span className="spacer" />
        {running ? (
          <>
            <button onClick={() => start()} disabled={starting} title="Start a fresh agent session">
              Restart
            </button>
            <button onClick={stop} disabled={starting}>
              Stop
            </button>
          </>
        ) : (
          <>
            {current?.resumeArgs?.length ? (
              <button
                onClick={() => start(harness, true)}
                disabled={!current.available || starting}
                title={harness === 'gemini' ? 'Continue the latest conversation in this project' : 'Choose a previous conversation of this project to continue'}
              >
                Resume
              </button>
            ) : null}
            <button className="primary" onClick={() => start()} disabled={!current?.available || starting} title="Start a new conversation">
              Start
            </button>
          </>
        )}
      </div>
      {error && <div className="error small">{error}</div>}
      <div className="terminal-host-wrap">
        <div className="terminal-host" ref={host} />
        {id === null && (
          <div className="terminal-empty">
            {intro ?? <p>Run an agent in this project's folder.</p>}
            <div className="harness-grid">
              {harnesses.map((h) => (
                <button
                  key={h.id}
                  disabled={!h.available || starting}
                  title={h.available ? `Runs ${h.command}` : `${h.command} is not on the PATH`}
                  onClick={() => (setHarness(h.id), start(h.id))}
                >
                  {h.name}
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
