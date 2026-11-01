// Syntax colors for a studio's language: CodeMirror tags mapped to the theme
// variables --syn-keyword, --syn-comment, --syn-string, --syn-number,
// --syn-atom, --syn-property, --syn-definition, --syn-type and
// --syn-function (styles.css has defaults). Tokenizers emit the usual
// token names ("keyword", "definition", …).
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { tags as t } from '@lezer/highlight';

export const studioHighlightStyle = HighlightStyle.define([
  { tag: t.keyword, color: 'var(--syn-keyword)', fontWeight: '600' },
  { tag: t.comment, color: 'var(--syn-comment)', fontStyle: 'italic' },
  { tag: t.string, color: 'var(--syn-string)' },
  { tag: t.number, color: 'var(--syn-number)' },
  { tag: t.unit, color: 'var(--syn-number)' },
  { tag: t.atom, color: 'var(--syn-atom)' },
  { tag: t.bool, color: 'var(--syn-atom)' },
  { tag: t.propertyName, color: 'var(--syn-property)' },
  { tag: t.definition(t.variableName), color: 'var(--syn-definition)', fontWeight: '600' },
  { tag: t.typeName, color: 'var(--syn-type)' },
  { tag: t.function(t.variableName), color: 'var(--syn-function)' },
  { tag: t.operator, color: 'var(--syn-keyword)' },
]);

/** The extension that applies studioHighlightStyle. */
export const studioHighlight = syntaxHighlighting(studioHighlightStyle);
