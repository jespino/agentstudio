// Saving a file the UI made (an export): the native save dialog in the
// desktop app, whose webview ignores <a download> (Go: studio.Base.SaveAs,
// served at /__studio/save), else a browser download.
import { hasNativeDialogs } from './project/actions';

/**
 * Saves data as name. Returns where it went (the path, or the name for a
 * download), or null if the user cancelled the dialog.
 */
export async function saveFile(data: Blob, name: string, title = 'Save'): Promise<string | null> {
  if (await hasNativeDialogs()) {
    const res = await fetch(`/__studio/save?name=${encodeURIComponent(name)}&title=${encodeURIComponent(title)}`, { method: 'POST', body: data });
    const out = (await res.json().catch(() => ({}))) as { saved?: boolean; path?: string; error?: string };
    if (!res.ok) throw new Error(out.error ?? `HTTP ${res.status}`);
    return out.saved ? (out.path ?? name) : null;
  }
  const a = document.createElement('a');
  a.href = URL.createObjectURL(data);
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 10000);
  return name;
}
