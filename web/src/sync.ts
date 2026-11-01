// Keeping a one-file project (Go: studio.Workspace) and the UI in step: the
// editor's text is saved shortly after typing stops, and the project and its
// file changing on disk (the agent's edits) reach the studio.
import { EVENT_PROJECT, EVENT_SOURCE, type Project, type SourceChange } from './api';
import { errorText } from './backend';
import { studioConfig } from './config';
import { toast } from './toast';

export interface SourceSaver {
  /** saves text after `delay` ms without another call */
  schedule: (text: string) => void;
  /** drops a pending save (the text was replaced from disk) */
  cancel: () => void;
  /** saves a pending text now */
  flush: () => void;
}

/**
 * Debounced saving; a failure shows a toast. save defaults to the
 * Workspace's SaveSource (pass one for other files: WriteSource(path, text)).
 */
export function createSourceSaver(delay = 400, save?: (text: string) => Promise<unknown>): SourceSaver {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let pending: string | null = null;
  const run = () => {
    clearTimeout(timer);
    if (pending === null) return;
    const text = pending;
    pending = null;
    (save ? save(text) : studioConfig().backend.call('SaveSource', text)).catch((e) => toast(`Could not save: ${errorText(e)}`));
  };
  return {
    schedule(text) {
      clearTimeout(timer);
      pending = text;
      timer = setTimeout(run, delay);
    },
    cancel: () => {
      clearTimeout(timer);
      pending = null;
    },
    flush: run,
  };
}

export interface WorkspaceHandlers<P extends Project> {
  /** the project opened, closed (null) or changed (its settings) */
  onProject: (project: P | null) => void;
  /** the open project's file changed on disk; text is its new content */
  onSource: (text: string) => void;
  /** the studio's view of the project, to ignore changes it made itself */
  current: () => { path?: string; source: string };
}

/** Listens to the project's events; returns the function that stops. */
export function onWorkspace<P extends Project = Project>(h: WorkspaceHandlers<P>): () => void {
  const { backend } = studioConfig();
  const offProject = backend.on<P | null>(EVENT_PROJECT, h.onProject);
  const offSource = backend.on<SourceChange>(EVENT_SOURCE, (ch) => {
    const cur = h.current();
    if (ch.path === cur.path && ch.source !== cur.source) h.onSource(ch.source);
  });
  return () => {
    offProject();
    offSource();
  };
}
