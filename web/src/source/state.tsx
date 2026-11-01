// Whether the source panel shows, and the line it should show: every studio
// has the panel, a toolbar button and Ctrl+2 to toggle it.
import { useEffect } from 'react';
import { create } from 'zustand';
import { storage, studioConfig } from '../config';

interface SourceUI {
  /** null until read from storage */
  open: boolean | null;
  /** the line to select (n makes repeated requests distinct) */
  reveal: { line: number; n: number } | null;
}

const useSourceUI = create<SourceUI>(() => ({ open: null, reveal: null }));
let revealN = 0;

function stored() {
  return storage.get('showSource', studioConfig().sourceOpen ? '1' : '0') === '1';
}

/** Whether the source panel shows. */
export function useSourceOpen(): boolean {
  const open = useSourceUI((s) => s.open);
  return open ?? stored();
}

/** The line the panel should select, for SourcePanel. */
export function useSourceReveal() {
  return useSourceUI((s) => s.reveal);
}

/** Shows (true), hides (false) or toggles the source panel. */
export function toggleSource(on?: boolean) {
  const open = on ?? !(useSourceUI.getState().open ?? stored());
  storage.set('showSource', open ? '1' : '0');
  useSourceUI.setState({ open });
}

/** Opens the source panel at a line (an error, a selected element). */
export function revealLine(line: number) {
  toggleSource(true);
  useSourceUI.setState({ reveal: { line, n: ++revealN } });
}

/** The toolbar button that toggles the source panel; it also binds Ctrl+2. */
export function SourceButton({ label = 'Source', title = 'Source panel (Ctrl+2)' }: { label?: string; title?: string }) {
  const open = useSourceOpen();
  useEffect(() => {
    const onKey = (ev: KeyboardEvent) => {
      if ((ev.ctrlKey || ev.metaKey) && !ev.altKey && !ev.shiftKey && ev.key === '2') {
        ev.preventDefault();
        toggleSource();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);
  return (
    <button className={open ? 'on' : ''} onClick={() => toggleSource()} title={title}>
      {label}
    </button>
  );
}
