import { useEffect, useRef } from 'react';
import { Annotation, EditorState, StateEffect, StateField, type Extension } from '@codemirror/state';
import { Decoration, EditorView, type DecorationSet } from '@codemirror/view';

/** A line to highlight (an error, a warning): its 1-based number and the CSS class. */
export interface LineMark {
  line: number;
  className: string;
}

const setMarks = StateEffect.define<LineMark[]>();
/** tags changes that come from outside, so they aren't echoed back as edits */
const external = Annotation.define<boolean>();

const lineMarks = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    deco = deco.map(tr.changes);
    for (const e of tr.effects) {
      if (!e.is(setMarks)) continue;
      const byLine = new Map<number, string>();
      for (const m of e.value) if (m.line >= 1 && m.line <= tr.state.doc.lines && !byLine.has(m.line)) byLine.set(m.line, m.className);
      deco = Decoration.set(
        [...byLine].sort((a, b) => a[0] - b[0]).map(([line, cls]) => Decoration.line({ class: cls }).range(tr.state.doc.line(line).from)),
      );
    }
    return deco;
  },
  provide: (f) => EditorView.decorations.from(f),
});

export interface SourceEditorProps {
  /** The source. Read when the editor mounts and whenever revision changes. */
  value: string;
  /** Bump it when the source is replaced from outside (the agent, another project). */
  revision: number;
  /** Called with the text on every edit made in the editor. */
  onEdit: (text: string) => void;
  /** The language, keymaps, line numbers… */
  extensions: Extension[];
  /** Lines to highlight; the first mark of a line wins. */
  marks?: LineMark[];
  /** Selects a line and scrolls to it (a new object each time). */
  reveal?: { line: number } | null;
  className?: string;
}

/** A CodeMirror editor kept in step with a source that also changes on disk. */
export function SourceEditor({ value, revision, onEdit, extensions, marks, reveal, className = 'source-editor' }: SourceEditorProps) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const latest = useRef({ value, onEdit });
  latest.current = { value, onEdit };

  useEffect(() => {
    view.current = new EditorView({
      state: EditorState.create({
        doc: latest.current.value,
        extensions: [
          ...extensions,
          lineMarks,
          EditorView.updateListener.of((u) => {
            if (u.docChanged && !u.transactions.some((tr) => tr.annotation(external))) latest.current.onEdit(u.state.doc.toString());
          }),
        ],
      }),
      parent: host.current!,
    });
    return () => {
      view.current?.destroy();
      view.current = null;
    };
  }, []);

  // the source was replaced from outside (disk, another project)
  useEffect(() => {
    const v = view.current;
    const source = latest.current.value;
    if (!v || v.state.doc.toString() === source) return;
    v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: source }, annotations: external.of(true) });
  }, [revision]);

  useEffect(() => {
    view.current?.dispatch({ effects: setMarks.of(marks ?? []) });
  }, [marks, revision]);

  useEffect(() => {
    const v = view.current;
    if (!v || !reveal || reveal.line < 1 || reveal.line > v.state.doc.lines) return;
    const line = v.state.doc.line(reveal.line);
    v.dispatch({ selection: { anchor: line.from, head: line.to }, effects: EditorView.scrollIntoView(line.from, { y: 'center' }) });
    v.focus();
  }, [reveal]);

  return <div className={className} ref={host} />;
}
