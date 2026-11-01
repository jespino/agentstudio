// Short messages at the bottom of the window: an error, "Saved …". Render
// <Toaster /> once; call toast() from anywhere.
import { create } from 'zustand';

interface ToastState {
  toast: { id: number; text: string } | null;
}

const useToast = create<ToastState>(() => ({ toast: null }));
let lastId = 0;

/** Shows text for a few seconds (a click hides it). */
export function toast(text: string, ms = 6000) {
  const id = ++lastId;
  useToast.setState({ toast: { id, text } });
  setTimeout(() => useToast.setState((s) => (s.toast?.id === id ? { toast: null } : s)), ms);
}

/** Hides the toast. */
export function dismissToast() {
  useToast.setState({ toast: null });
}

/** Where toasts show (class "toast"). */
export function Toaster() {
  const t = useToast((s) => s.toast);
  if (!t) return null;
  return (
    <div className="toast" key={t.id} role="status" onClick={dismissToast}>
      {t.text}
    </div>
  );
}
