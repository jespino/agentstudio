import { useEffect, useState } from 'react';
import type { DirListing } from '../api';
import { errorText } from '../backend';
import { capitalized, label, studioConfig, useStudioUI } from '../config';
import { createProject, openProject } from './actions';

/** In-app folder browser for opening or creating a project (shells without native dialogs). */
export function ProjectPicker() {
  const mode = useStudioUI((s) => s.picker);
  const template = useStudioUI((s) => s.template);
  const [listing, setListing] = useState<DirListing | null>(null);
  const [path, setPath] = useState('');
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const c = studioConfig();

  const go = async (p: string) => {
    try {
      const l = await c.api.listDir(p);
      setListing(l);
      setPath(l.path);
      setError(null);
    } catch (e) {
      setError(errorText(e));
    }
  };

  useEffect(() => {
    // Reset on close too: the picker stays mounted, and clearing only on
    // open would show (and append typing to) the previous name for a frame.
    setName('');
    setError(null);
    if (!mode) return;
    const dir = c.currentDir?.();
    go(dir ? dir.replace(/[\\/][^\\/]+$/, '') : '');
  }, [mode]);

  if (!mode) return null;
  const close = () => useStudioUI.setState({ picker: null, template: null });
  const sep = path.includes('\\') ? '\\' : '/';
  const target = name.trim() ? `${path}${path.endsWith(sep) ? '' : sep}${name.trim()}` : path;
  const entries = listing?.entries.filter((e) => e.isDir || (mode === 'open' && e.isScript)) ?? [];
  const openable = listing ? (c.canOpen ? c.canOpen(listing) : listing.isProject) : false;
  const noun = c.noun ?? 'project';

  return (
    <div className="modal-backdrop" onMouseDown={close}>
      <div className="modal" onMouseDown={(e) => e.stopPropagation()}>
        <h2>{mode === 'new' ? (template ? `${label('New')} from “${template.name}”` : label('New')) : label('Open')}</h2>
        {(mode === 'new' ? c.newHint : c.openHint) && <p className="muted small">{mode === 'new' ? c.newHint : c.openHint}</p>}
        <div className="picker-path">
          <button onClick={() => listing?.parent && go(listing.parent)} disabled={!listing?.parent} title="Parent folder">
            ↑
          </button>
          <input value={path} onChange={(e) => setPath(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && go(path)} />
          <button onClick={() => go(path)}>Go</button>
        </div>
        <ul className="picker-list">
          {entries.map((e) => (
            <li
              key={e.path}
              className={e.isProject || e.isScript ? 'project' : ''}
              onClick={() => (e.isScript || (mode === 'open' && e.isProject && c.openProjectFolders) ? openProject(e.path) : go(e.path))}
              onDoubleClick={() => mode === 'open' && e.isProject && openProject(e.path)}
              title={e.path}
            >
              <span className="icon">{e.isScript ? '◆' : e.isProject ? '▣' : '▸'}</span>
              <span className="ellipsis">{e.name}</span>
              {e.isProject && <span className="tag">{noun}</span>}
            </li>
          ))}
          {entries.length === 0 && <li className="muted small">Nothing here.</li>}
        </ul>
        {error && <div className="danger-text small">{error}</div>}
        {mode === 'new' ? (
          <>
            <label className="field">
              {capitalized()} folder name (created inside the current folder; leave empty to use the current folder)
              <input
                value={name}
                placeholder={c.namePlaceholder ?? `my-${noun}`}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && createProject(target)}
              />
            </label>
            <div className="row">
              <span className="muted small ellipsis" title={target}>
                {target}
              </span>
              <span className="spacer" />
              <button onClick={close}>Cancel</button>
              <button className="primary" onClick={() => createProject(target)} disabled={!path}>
                {label('Create')}
              </button>
            </div>
          </>
        ) : (
          <div className="row">
            <span className="spacer" />
            <button onClick={close}>Cancel</button>
            <button className="primary" onClick={() => openProject(path)} disabled={!openable} title={openable ? '' : (c.notOpenable ?? `This folder has no ${noun}`)}>
              Open this folder
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
