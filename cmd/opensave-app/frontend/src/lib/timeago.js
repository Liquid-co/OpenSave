// A stored moment, said the way a person would: "just now", "4 min ago",
// "yesterday". The same thresholds as the CLI's timeAgo, so the app and the
// terminal describe one sync the same way.
//
// Beyond a week the date itself is clearer than a count of days. Anything
// unparseable is returned as it came, so a wrong-looking value can be
// reported instead of vanishing.

const SEC = 1000;
const MIN = 60 * SEC;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;

export function timeAgo(iso, now = Date.now()) {
  if (!iso) return 'never';
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return String(iso);
  const d = now - t;
  // A stamp a few seconds in the future is a peer's clock running slightly
  // ahead, not a negative age.
  if (d < 45 * SEC) return 'just now';
  if (d < 90 * SEC) return 'a minute ago';
  if (d < HOUR) return `${Math.round(d / MIN)} min ago`;
  if (d < 90 * MIN) return 'an hour ago';
  if (d < DAY) return `${Math.round(d / HOUR)} hours ago`;
  if (d < 36 * HOUR) return 'yesterday';
  if (d < 7 * DAY) return `${Math.round(d / DAY)} days ago`;
  return 'on ' + new Date(t).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
}

// The newest of several stamps, or null when there are none. Used where one
// game gets one line: "synced 4 min ago" is the most recent device.
export function latestOf(stamps) {
  let best = null;
  for (const iso of Object.values(stamps ?? {})) {
    if (iso && (best === null || iso > best)) best = iso;
  }
  return best;
}
