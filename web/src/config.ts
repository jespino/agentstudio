// What the shared components need to know about the studio they run in.
import { create } from 'zustand';
import { baseApi, type BaseApi, type DirListing, type Template } from './api';
import { errorText, type Backend } from './backend';
import { toast } from './toast';

export interface StudioConfig {
  backend: Backend;
  /** Short name: prefixes what the UI keeps in localStorage ("kino" → "kino:harness"). */
  id: string;
  /** What a project is called in the UI: "project", "trip". */
  noun?: string;
  /** The native folder dialogs' titles. */
  openTitle?: string;
  newTitle?: (template: Template | null) => string;
  /** The folder browser's explanations and the new folder's placeholder name. */
  openHint?: string;
  newHint?: string;
  namePlaceholder?: string;
  /** Says why "Open this folder" is disabled: "This folder has no .kino file". */
  notOpenable?: string;
  /** Opens a project file or folder. Default: OpenProject(path). */
  open?: (path: string) => Promise<unknown>;
  /** Creates a project in dir. Default: NewProject(dir, name || template name, template source). */
  create?: (dir: string, name: string, template: Template | null) => Promise<unknown>;
  /** A click on a project folder in the folder browser opens it (instead of going in; a double click always opens). */
  openProjectFolders?: boolean;
  /** Whether the folder browser can open the folder it shows. Default: it holds project files. */
  canOpen?: (listing: DirListing) => boolean;
  /** The open project's folder: the folder browser starts next to it. */
  currentDir?: () => string | undefined;
  /** Whether the source panel shows the first time (the user's choice is kept). Default: hidden. */
  sourceOpen?: boolean;
  /** Shows an error message. Default: a toast (render <Toaster />). */
  notify?: (message: string) => void;
}

let config: (StudioConfig & { api: BaseApi }) | null = null;

/** Configures the shared components; call it once, before rendering. */
export function configureStudio(c: StudioConfig) {
  config = { ...c, api: baseApi(c.backend.call) };
}

export function studioConfig() {
  if (!config) throw new Error('agentstudio: call configureStudio() before rendering its components');
  return config;
}

/** The noun, in a label: noun('New', '…') → "New project…". */
export function label(before: string, after = '') {
  return `${before} ${studioConfig().noun ?? 'project'}${after}`;
}

export function capitalized() {
  const n = studioConfig().noun ?? 'project';
  return n[0].toUpperCase() + n.slice(1);
}

/** The shared components' UI state. */
interface StudioUI {
  /** the in-app folder browser (shells without native dialogs) */
  picker: 'open' | 'new' | null;
  /** the template a new project starts from */
  template: Template | null;
}

export const useStudioUI = create<StudioUI>(() => ({ picker: null, template: null }));

export function notify(e: unknown) {
  const msg = errorText(e);
  const n = config?.notify;
  if (n) n(msg);
  else toast(msg);
}

/** localStorage, which a private window or a sandbox may refuse. */
export const storage = {
  get(key: string, fallback: string) {
    try {
      return localStorage.getItem(`${studioConfig().id}:${key}`) ?? fallback;
    } catch {
      return fallback;
    }
  },
  set(key: string, value: string) {
    try {
      localStorage.setItem(`${studioConfig().id}:${key}`, value);
    } catch {
      /* storage unavailable */
    }
  },
};
