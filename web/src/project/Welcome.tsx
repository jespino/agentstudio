import { useEffect, useState, type ReactNode } from 'react';
import type { RecentProject, Template } from '../api';
import { label, studioConfig } from '../config';
import { newProjectFlow, openProject, openProjectFlow } from './actions';

/** Templates offered on the welcome screen, under a heading. */
export interface TemplateGroup {
  /** a subheading (none for a single group) */
  label?: string;
  templates: Template[];
  /** shown when there are no templates (none: the group is hidden) */
  empty?: ReactNode;
}

export interface WelcomeProps {
  /** The studio's name. */
  title: string;
  /** What the studio does, and what a project is. */
  children?: ReactNode;
  /** "Start from a template", and the groups under it. */
  templatesTitle?: string;
  templates?: TemplateGroup[];
}

function TemplateGrid({ templates }: { templates: Template[] }) {
  return (
    <div className="example-grid">
      {templates.map((t) => (
        <button key={t.name} className="small" onClick={() => newProjectFlow(t)} title={`${label('New')} from ${t.name}`}>
          {t.name}
        </button>
      ))}
    </div>
  );
}

/** Shown when no project is open: new, open, recent and templates. */
export function Welcome({ title, children, templatesTitle = 'Start from a template', templates = [] }: WelcomeProps) {
  const [recent, setRecent] = useState<RecentProject[]>([]);
  useEffect(() => {
    studioConfig()
      .api.recentProjects()
      .then((r) => setRecent(r ?? []), () => {});
  }, []);
  const groups = templates.filter((g) => g.templates.length > 0 || g.empty);
  return (
    <div className="welcome">
      <div className="welcome-card">
        <h1>{title}</h1>
        {children && <div className="muted">{children}</div>}
        <div className="welcome-actions">
          <button className="primary big" onClick={() => newProjectFlow()}>
            {label('New', '…')}
          </button>
          <button className="big" onClick={openProjectFlow}>
            {label('Open', '…')}
          </button>
        </div>
        {recent.length > 0 && (
          <>
            <h3>Recent</h3>
            <ul className="recent">
              {recent.map((r) => (
                <li key={r.path}>
                  <button className="link" disabled={!r.exists} onClick={() => openProject(r.path)} title={r.path}>
                    <span className="recent-name">{r.name}</span>
                    <span className="muted small ellipsis">{r.exists ? r.path : 'missing'}</span>
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}
        {groups.length > 0 && <h3>{templatesTitle}</h3>}
        {groups.map((g, i) => (
          <div key={g.label ?? i}>
            {g.label && <h4>{g.label}</h4>}
            {g.templates.length > 0 ? <TemplateGrid templates={g.templates} /> : <p className="muted small">{g.empty}</p>}
          </div>
        ))}
      </div>
    </div>
  );
}
