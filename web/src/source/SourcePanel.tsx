import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Prec, type Extension } from '@codemirror/state';
import { keymap, type EditorView } from '@codemirror/view';
import { errorText } from '../backend';
import { SourceEditor, type LineMark } from './SourceEditor';
import { revealLine, toggleSource, useSourceReveal } from './state';

/** A compile error shown above the editor; a click selects its line. */
export interface SourceProblem {
  line?: number;
  col?: number;
  message: string;
}

export interface SourcePanelProps {
  /** The source as the studio has it. */
  value: string;
  /** Bump it when value is replaced from outside (the agent's edit, another file). */
  revision: number;
  /** The language and editing extensions. */
  extensions: Extension[];
  /** The file, for the header. */
  file?: string;
  /** Several files (folder projects): a chooser in the header. */
  files?: { list: string[]; selected: string; onSelect: (path: string) => void };
  /** Lines to highlight (errors, warnings). */
  marks?: LineMark[];
  /** The compile error, above the editor. */
  problem?: SourceProblem | null;
  /** Every edit made in the editor (save it as you type, validate it…). */
  onEdit?: (text: string) => void;
  /**
   * Apply mode: edits are kept until applied (Apply, Ctrl+S) or reverted, and
   * the source changing on disk doesn't overwrite them. Resolves to null when
   * applied, or to why not.
   */
  onApply?: (text: string) => Promise<string | null>;
  /** The CodeMirror view (null when it goes): diagnostics, selections. */
  onView?: (view: EditorView | null) => void;
  /** The panel's title. */
  title?: string;
  /** Next to the file name ("saved automatically" by default when not applying). */
  status?: ReactNode;
}

/**
 * The source panel every studio has: the project's text, kept in step with
 * the disk, where the agent edits it too. Shown with SourceButton / Ctrl+2.
 */
export function SourcePanel(p: SourcePanelProps) {
  const apply = p.onApply;
  const view = useRef<EditorView | null>(null);
  const synced = useRef(p.value); // the source the editor last matched
  const [dirty, setDirty] = useState(false);
  const [message, setMessage] = useState('');
  const reveal = useSourceReveal();
  const applyRef = useRef<() => void>(() => {});

  // a new source from outside: it is the editor's unless edits are kept
  useEffect(() => {
    if (!apply) {
      synced.current = p.value;
      return;
    }
    const v = view.current;
    const current = v ? v.state.doc.toString() : synced.current;
    const wasClean = current === synced.current;
    synced.current = p.value;
    if (wasClean || current === p.value) {
      setDirty(false);
      setMessage('');
    } else setMessage('The source changed while you were editing; Revert to load it');
  }, [p.revision, p.value]);

  const doApply = async () => {
    const v = view.current;
    if (!apply || !v) return;
    const text = v.state.doc.toString();
    try {
      const why = await apply(text);
      if (why === null) {
        synced.current = text;
        setDirty(false);
        setMessage('Applied');
      } else setMessage(why);
    } catch (e) {
      setMessage(errorText(e));
    }
  };
  applyRef.current = doApply;

  const revert = () => {
    const v = view.current;
    if (!v) return;
    v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: p.value } });
    synced.current = p.value;
    setDirty(false);
    setMessage('');
  };

  const extensions = useMemo(
    () => (apply ? [Prec.high(keymap.of([{ key: 'Mod-s', run: () => (applyRef.current(), true) }])), ...p.extensions] : p.extensions),
    [],
  );

  const onEdit = (text: string) => {
    if (apply) {
      setDirty(text !== synced.current);
      setMessage((m) => (m.startsWith('The source changed') ? m : ''));
    }
    p.onEdit?.(text);
  };

  const pr = p.problem;
  return (
    <div className="panel source">
      <div className="panel-header">
        <span className="panel-title">{p.title ?? 'Source'}</span>
        {p.files ? (
          <select value={p.files.selected} onChange={(e) => p.files!.onSelect(e.target.value)} title="The file shown">
            {p.files.list.map((f) => (
              <option key={f} value={f}>
                {f}
              </option>
            ))}
          </select>
        ) : (
          p.file && <span className="muted small ellipsis">{p.file}</span>
        )}
        <span className="muted small ellipsis">{apply ? [dirty ? '● modified' : '', message].filter(Boolean).join(' · ') : (p.status ?? 'saved automatically')}</span>
        <span className="spacer" />
        {apply && (
          <>
            <button className="small" onClick={revert} disabled={!dirty && !message.startsWith('The source changed')}>
              Revert
            </button>
            <button className="small primary" onClick={doApply} disabled={!dirty} title="Apply (Ctrl+S)">
              Apply
            </button>
          </>
        )}
        <button className="small" onClick={() => toggleSource(false)} title="Hide the source (Ctrl+2)">
          ✕
        </button>
      </div>
      {pr && (
        <button className="source-error" onClick={() => pr.line && revealLine(pr.line)} title={pr.line ? 'Go to the error' : undefined}>
          {pr.line ? `${pr.line}:${pr.col ?? 1} ` : ''}
          {pr.message}
        </button>
      )}
      <SourceEditor
        key={p.files?.selected}
        value={p.value}
        revision={p.revision}
        onEdit={onEdit}
        extensions={extensions}
        marks={p.marks}
        reveal={reveal}
        keepEdits={apply ? () => view.current !== null && view.current.state.doc.toString() !== synced.current : undefined}
        onView={(v) => {
          view.current = v;
          p.onView?.(v);
        }}
      />
    </div>
  );
}
