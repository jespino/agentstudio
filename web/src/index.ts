// The frontend of an agent studio. Configure it once (configureStudio),
// then use its components; import "@jespino/agentstudio/styles.css" too.
export { createBackend, errorText, isWails, type Backend, type BackendOptions } from './backend';
export {
  baseApi,
  workspaceApi,
  EVENT_PROJECT,
  EVENT_SOURCE,
  EVENT_TERMINAL_DATA,
  EVENT_TERMINAL_EXIT,
  EVENT_TERMINAL_START,
  type BaseApi,
  type DirEntry,
  type DirListing,
  type Harness,
  type Project,
  type RecentProject,
  type SourceChange,
  type Template,
  type TerminalInfo,
} from './api';
export { configureStudio, studioConfig, useStudioUI, type StudioConfig } from './config';
export {
  agentInput,
  askActiveAgent,
  focusAgentPanel,
  getActiveAgent,
  hasActiveAgent,
  setActiveAgent,
  setTerminalSink,
  subscribeToAgent,
  typeInTerminal,
} from './terminal/bus';
export { TerminalPanel, type TerminalPanelProps } from './terminal/TerminalPanel';
export { closeProject, createProject, hasNativeDialogs, newProjectFlow, openProject, openProjectFlow } from './project/actions';
export { Welcome, type TemplateGroup, type WelcomeProps } from './project/Welcome';
export { ProjectPicker } from './project/ProjectPicker';
export { ProjectMenu, type ProjectMenuProps } from './project/ProjectMenu';
export { SourceEditor, type LineMark, type SourceEditorProps } from './source/SourceEditor';
export { SourcePanel, type SourcePanelProps, type SourceProblem } from './source/SourcePanel';
export { revealLine, SourceButton, toggleSource, useSourceOpen } from './source/state';
export { StudioLayout, type StudioLayoutProps } from './layout/StudioLayout';
export { askAgent, useAskAgent, type AskAgent } from './terminal/ask';
export { dismissToast, toast, Toaster } from './toast';
export { saveFile } from './save';
export { createSourceSaver, onWorkspace, type SourceSaver, type WorkspaceHandlers } from './sync';
export { studioHighlight, studioHighlightStyle } from './source/highlight';
