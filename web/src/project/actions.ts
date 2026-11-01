// Project flows: native dialogs in the desktop app, the in-app folder
// browser elsewhere (`serve` in a browser).
import type { Template } from '../api';
import { label, notify, studioConfig, useStudioUI } from '../config';

let native: Promise<boolean> | null = null;
/** Whether the shell has native dialogs (the desktop app). */
export const hasNativeDialogs = () => (native ??= studioConfig().api.hasNativeDialogs().catch(() => false));

async function guarded(fn: () => Promise<unknown>) {
  try {
    await fn();
  } catch (e) {
    notify(e);
  }
}

/** Opens a project file or folder. */
export function openProject(path: string) {
  return guarded(async () => {
    const c = studioConfig();
    await (c.open ? c.open(path) : c.backend.call('OpenProject', path));
    useStudioUI.setState({ picker: null });
  });
}

/** Creates the project in dir (from the chosen template, if any). */
export function createProject(dir: string, name = '') {
  const template = useStudioUI.getState().template;
  return guarded(async () => {
    const c = studioConfig();
    if (c.create) await c.create(dir, name, template);
    else await c.backend.call('NewProject', dir, name || template?.name || '', template?.source ?? '');
    useStudioUI.setState({ picker: null, template: null });
  });
}

/** "Open project…": a native folder dialog, or the in-app browser. */
export function openProjectFlow() {
  return guarded(async () => {
    if (await hasNativeDialogs()) {
      const dir = await studioConfig().api.chooseDirectory(studioConfig().openTitle ?? label('Open a', ' folder'), false);
      if (dir) await openProject(dir);
    } else {
      useStudioUI.setState({ picker: 'open' });
    }
  });
}

/** "New project…": choose or create the folder that will hold it. */
export function newProjectFlow(template: Template | null = null) {
  useStudioUI.setState({ template });
  return guarded(async () => {
    if (await hasNativeDialogs()) {
      const c = studioConfig();
      const title = c.newTitle
        ? c.newTitle(template)
        : template
          ? `Choose a folder for the “${template.name}” ${c.noun ?? 'project'}`
          : label('Choose or create a folder for the new');
      const dir = await c.api.chooseDirectory(title, true);
      if (dir) await createProject(dir);
      else useStudioUI.setState({ template: null });
    } else {
      useStudioUI.setState({ picker: 'new' });
    }
  });
}

/** Closes the project (back to the welcome screen). */
export function closeProject() {
  return guarded(() => studioConfig().backend.call('CloseProject'));
}
