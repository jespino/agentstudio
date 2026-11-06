// Lets other panels type into the active terminal (e.g. "@media/clip.mp4"
// from the Media panel).
import { toggleAgent } from '../layout/agent';

type Sink = (text: string) => boolean;
let sink: Sink | null = null;

export function setTerminalSink(fn: Sink | null) {
  sink = fn;
}

/** Types text into the active terminal. Returns false if none is running. */
export function typeInTerminal(text: string): boolean {
  if (!sink) return false;
  return sink(text);
}

interface AgentConnection {
  send: (prompt: string) => Promise<void>;
  focus: () => void;
}

let agent: AgentConnection | null = null;
const listeners = new Set<() => void>();

export function setActiveAgent(connection: AgentConnection | null) {
  agent = connection;
  listeners.forEach((fn) => fn());
}

export function hasActiveAgent() {
  return agent !== null;
}
export function getActiveAgent() {
  return agent;
}
export function subscribeToAgent(fn: () => void) {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

/** Paste the whole prompt as one message, then submit it to the agent CLI. */
export function agentInput(prompt: string): string {
  // Error text may include terminal control sequences. Strip controls so it
  // cannot end bracketed paste early or issue terminal commands.
  const text = prompt.replace(/[\x00-\x08\x0b-\x1f\x7f-\x9f]/g, '');
  return `\x1b[200~${text}\x1b[201~\r`;
}

export async function askActiveAgent(prompt: string): Promise<boolean> {
  const current = agent;
  if (!current) return false;
  toggleAgent(true); // what it answers must be seen
  await current.send(prompt);
  current.focus();
  return true;
}

export function focusAgentPanel() {
  toggleAgent(true);
  // a folded panel can't take the focus until it has been drawn open
  requestAnimationFrame(() => {
    const panel = document.querySelector<HTMLElement>('.terminal');
    panel?.scrollIntoView({ block: 'nearest' });
    panel?.querySelector<HTMLSelectElement>('select')?.focus();
  });
}
