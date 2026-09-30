/** Go encodes an unset time.Time as year 1; treat that as "never". */
export function isSet(iso: string | undefined): iso is string {
  return !!iso && !iso.startsWith("0001-01-01");
}

export function timeAgo(iso: string | undefined, now: Date = new Date()): string {
  if (!isSet(iso)) return "never";
  const seconds = Math.max(0, Math.round((now.getTime() - new Date(iso).getTime()) / 1000));
  if (seconds < 5) return "just now";
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

export function formatTime(iso: string | undefined): string {
  if (!isSet(iso)) return "—";
  return new Date(iso).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}
