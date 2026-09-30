import { useCallback, useState } from "react";

export interface Toast {
  id: number;
  tone: "ok" | "bad";
  message: string;
}

let nextId = 1;

/** Short-lived notifications for the result of user actions. */
export function useToasts() {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const push = useCallback((tone: Toast["tone"], message: string) => {
    const id = nextId++;
    setToasts((list) => [...list, { id, tone, message }]);
    setTimeout(() => setToasts((list) => list.filter((t) => t.id !== id)), 4500);
  }, []);

  return { toasts, push };
}

export function Toasts({ toasts }: { toasts: Toast[] }) {
  return (
    <div className="toasts" aria-live="polite">
      {toasts.map((t) => (
        <div key={t.id} className={`toast toast-${t.tone}`}>
          {t.message}
        </div>
      ))}
    </div>
  );
}
