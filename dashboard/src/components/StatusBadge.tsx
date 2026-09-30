export type Tone = "ok" | "warn" | "bad" | "muted";

/** A colored pill that always carries a text label, so state never relies on color alone. */
export function StatusBadge({ tone, label }: { tone: Tone; label: string }) {
  return (
    <span className={`badge badge-${tone}`}>
      <span className="badge-dot" aria-hidden="true" />
      {label}
    </span>
  );
}

export function healthTone(status: string): Tone {
  switch (status) {
    case "Healthy":
      return "ok";
    case "Unhealthy":
      return "bad";
    default:
      return "muted";
  }
}
