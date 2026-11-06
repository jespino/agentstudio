// Whether the agent panel shows: a studio whose StudioLayout is `collapsible`
// can fold it to a rail on the right, with a toolbar button and Ctrl+3. Asking
// the agent opens it again.
import { createContext, useContext, useEffect } from 'react';
import { create } from 'zustand';
import { storage } from '../config';

const useAgentUI = create<{ open: boolean | null }>(() => ({ open: null }));

function stored() {
  return storage.get('showAgent', '1') === '1';
}

/** Whether the agent panel is open. */
export function useAgentOpen(): boolean {
  const open = useAgentUI((s) => s.open);
  return open ?? stored();
}

/** Whether the agent panel is open, outside React. */
export function isAgentOpen() {
  return useAgentUI.getState().open ?? stored();
}

/** Opens (true), folds (false) or toggles the agent panel. */
export function toggleAgent(on?: boolean) {
  const open = on ?? !isAgentOpen();
  storage.set('showAgent', open ? '1' : '0');
  useAgentUI.setState({ open });
}

/** Whether the agent panel around a component can fold (StudioLayout). */
export const AgentFolds = createContext(false);

/** A panel with its bar on the right; the arrow says which way it will go. */
export function AgentFoldIcon({ fold }: { fold: boolean }) {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <rect x="1.5" y="2.5" width="13" height="11" rx="1.5" />
      <path d="M10.5 2.5v11" />
      <path d={fold ? 'M5.5 6 7.5 8l-2 2' : 'M7.5 6l-2 2 2 2'} />
    </svg>
  );
}

/** The icon in the agent panel's header that folds it. */
export function AgentFoldButton() {
  if (!useContext(AgentFolds)) return null;
  return (
    <button className="agent-fold" onClick={() => toggleAgent(false)} title="Hide the agent (Ctrl+3)" aria-label="Hide the agent">
      <AgentFoldIcon fold />
    </button>
  );
}

/** The toolbar button that toggles the agent panel; it also binds Ctrl+3. */
export function AgentButton({ label = 'Agent', title = 'Agent panel (Ctrl+3)' }: { label?: string; title?: string }) {
  const open = useAgentOpen();
  useEffect(() => {
    const onKey = (ev: KeyboardEvent) => {
      if ((ev.ctrlKey || ev.metaKey) && !ev.altKey && !ev.shiftKey && ev.key === '3') {
        // before the terminal, where Ctrl+3 would be a character for the agent
        ev.preventDefault();
        ev.stopPropagation();
        toggleAgent();
      }
    };
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  }, []);
  return (
    <button className={open ? 'on' : ''} onClick={() => toggleAgent()} title={title}>
      {label}
    </button>
  );
}
