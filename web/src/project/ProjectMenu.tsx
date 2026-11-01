import { useEffect, useRef, useState, type ReactNode } from 'react';
import type { RecentProject, Template } from '../api';
import { label, studioConfig } from '../config';
import { closeProject, newProjectFlow, openProject, openProjectFlow } from './actions';

export interface ProjectMenuProps {
  /** The open project (its name and file). */
  project: { name: string; path: string } | null;
  /** Template sections ("New from template"), loaded when the menu opens. */
  templates?: () => { label: string; templates: Template[] }[] | Promise<{ label: string; templates: Template[] }[]>;
  /** Extra items, before "Close". */
  children?: ReactNode;
  /** Shows the file name after the project name (one-file projects). */
  showFile?: boolean;
}

/** Toolbar project name with New / Open / Recent / templates / Close. */
export function ProjectMenu({ project, templates, children, showFile = true }: ProjectMenuProps) {
  const [open, setOpen] = useState(false);
  const [recent, setRecent] = useState<RecentProject[]>([]);
  const [sections, setSections] = useState<{ label: string; templates: Template[] }[]>([]);
  const box = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    if (!open) return;
    studioConfig()
      .api.recentProjects()
      .then((r) => setRecent(r ?? []), () => {});
    if (templates) Promise.resolve(templates()).then(setSections, () => {});
    const close = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    window.addEventListener('mousedown', close);
    return () => window.removeEventListener('mousedown', close);
  }, [open]);

  const file = project?.path.split(/[\\/]/).pop() ?? '';
  const pick = (fn: () => unknown) => () => {
    setOpen(false);
    fn();
  };
  const others = recent.filter((r) => r.exists && r.path !== project?.path).slice(0, 8);
  return (
    <span className="project-menu" ref={box}>
      <button className="link project-name" onClick={() => setOpen(!open)} title={project?.path}>
        {project?.name ?? label('No')}
        {showFile && file && <span className="muted"> / {file}</span>} ▾
      </button>
      {open && (
        <div className="menu" onClick={(e) => (e.target as HTMLElement).closest('button') && setOpen(false)}>
          <button onClick={pick(() => newProjectFlow())}>{label('New', '…')}</button>
          <button onClick={pick(openProjectFlow)}>{label('Open', '…')}</button>
          {others.length > 0 && <div className="menu-sep">Recent</div>}
          {others.map((r) => (
            <button key={r.path} onClick={pick(() => openProject(r.path))} title={r.path}>
              {r.name}
            </button>
          ))}
          {sections.map(
            (s) =>
              s.templates.length > 0 && (
                <div key={s.label}>
                  <div className="menu-sep">{s.label}</div>
                  <div className="menu-examples">
                    {s.templates.map((t) => (
                      <button key={t.name} onClick={pick(() => newProjectFlow(t))}>
                        {t.name}
                      </button>
                    ))}
                  </div>
                </div>
              ),
          )}
          {children}
          <div className="menu-sep" />
          <button onClick={pick(closeProject)}>{label('Close')}</button>
        </div>
      )}
    </span>
  );
}
