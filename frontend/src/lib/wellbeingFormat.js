/**
 * Formatting helpers for Digital Wellbeing analytics UI.
 */

export function formatDuration(totalSeconds) {
  const sec = Math.max(0, Math.floor(Number(totalSeconds) || 0));
  if (sec === 0) return "0m";
  if (sec < 60) return `${sec}s`;
  const totalMinutes = Math.floor(sec / 60);
  const hours = Math.floor(totalMinutes / 60);
  const mins = totalMinutes % 60;
  if (hours === 0) return `${mins}m`;
  return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
}

export function formatDelta(deltaSeconds) {
  const d = Math.floor(Number(deltaSeconds) || 0);
  if (Math.abs(d) < 60) return "Even with yesterday";
  const sign = d > 0 ? "+" : "-";
  return `${sign}${formatDuration(Math.abs(d))} vs prior`;
}

export function formatShortDay(dateStr) {
  if (!dateStr || typeof dateStr !== "string") return "";
  const parts = dateStr.split("-");
  if (parts.length !== 3) return dateStr;
  const d = new Date(Number(parts[0]), Number(parts[1]) - 1, Number(parts[2]));
  if (Number.isNaN(d.getTime())) return dateStr;
  return d.toLocaleDateString(undefined, { weekday: "short" });
}
