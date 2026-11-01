// The methods every studio's backend has (Go: studio.Base and
// studio.Workspace), and the types they exchange.

export interface Harness {
  id: string;
  name: string;
  command: string;
  args?: string[];
  /** added to args to continue a saved conversation; none when the agent can't */
  resumeArgs?: string[];
  available: boolean;
}

export interface TerminalInfo {
  id: string;
  harness: string;
  running: boolean;
}

export interface RecentProject {
  /** the project file (the folder, for folder projects) */
  path: string;
  name: string;
  opened: string;
  exists: boolean;
}

export interface DirEntry {
  name: string;
  path: string;
  isDir: boolean;
  /** a folder holding project files */
  isProject: boolean;
  /** a project file */
  isScript: boolean;
  /** another file the studio lists (media, …) */
  isOther: boolean;
}

export interface DirListing {
  path: string;
  parent?: string;
  /** the folder holds project files */
  isProject: boolean;
  /** its project files */
  scripts: string[];
  entries: DirEntry[];
}

/** The open project of a one-file studio (Go: studio.Workspace). */
export interface Project {
  /** the project file */
  path: string;
  /** the project folder */
  dir: string;
  /** the folder's name */
  name: string;
  source: string;
}

/** The project file's new content after an edit on disk. */
export interface SourceChange {
  path: string;
  source: string;
}

/** A template a new project starts from. */
export interface Template {
  name: string;
  source: string;
  /** a template the backend knows by name (custom templates) */
  custom?: boolean;
}

export const EVENT_PROJECT = 'project:changed';
export const EVENT_SOURCE = 'project:source';
export const EVENT_TERMINAL_DATA = 'terminal:data';
export const EVENT_TERMINAL_EXIT = 'terminal:exit';
export const EVENT_TERMINAL_START = 'terminal:start';

type Call = <T>(method: string, ...args: unknown[]) => Promise<T>;

/** The shared methods (studio.Base), for a backend's call. */
export function baseApi(call: Call) {
  return {
    recentProjects: () => call<RecentProject[] | null>('RecentProjects'),
    listDir: (path: string) => call<DirListing>('ListDir', path),
    hasNativeDialogs: () => call<boolean>('HasNativeDialogs'),
    chooseDirectory: (title: string, canCreate: boolean) => call<string>('ChooseDirectory', title, canCreate),
    harnesses: () => call<Harness[]>('Harnesses'),
    startTerminal: (harness: string, cols: number, rows: number) => call<string>('StartTerminal', harness, cols, rows),
    resumeTerminal: (harness: string, cols: number, rows: number) => call<string>('ResumeTerminal', harness, cols, rows),
    terminalWrite: (id: string, data: string) => call<void>('TerminalWrite', id, data),
    terminalResize: (id: string, cols: number, rows: number) => call<void>('TerminalResize', id, cols, rows),
    terminalBuffer: (id: string) => call<string>('TerminalBuffer', id),
    stopTerminal: (id: string) => call<void>('StopTerminal', id),
    terminals: () => call<TerminalInfo[] | null>('Terminals'),
    /** the project's source files (relative paths), for folder projects */
    sourceFiles: () => call<string[]>('SourceFiles'),
    readSource: (path: string) => call<string>('ReadSource', path),
    writeSource: (path: string, text: string) => call<void>('WriteSource', path, text),
  };
}

/** The shared methods plus those of a one-file studio (studio.Workspace). */
export function workspaceApi<P extends Project = Project>(call: Call) {
  return {
    ...baseApi(call),
    currentProject: () => call<P | null>('CurrentProject'),
    openProject: (path: string) => call<P>('OpenProject', path),
    newProject: (dir: string, name: string, source: string) => call<P>('NewProject', dir, name, source),
    closeProject: () => call<void>('CloseProject'),
    saveSource: (source: string) => call<void>('SaveSource', source),
  };
}

export type BaseApi = ReturnType<typeof baseApi>;
