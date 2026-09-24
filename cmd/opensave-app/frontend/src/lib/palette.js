// Ranking for the Ctrl+K palette: which entries match what was typed, and
// in what order.
//
// Every word typed has to be found in an entry, at the start of one of its
// words or anywhere inside it, so "snap had" finds "Snapshot Hades" and
// "elden" finds "Elden Ring". Entries whose label starts with what was typed
// come first; among equals, a game comes before an action on it and a
// shorter label before a longer one.

const words = (s) => s.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter(Boolean);

/** How well an entry matches, or null when some typed word is missing. */
export function score(entry, query) {
  const q = query.trim().toLowerCase();
  if (!q) return 0;
  const label = entry.label.toLowerCase();
  const haystack = `${label} ${(entry.keywords ?? []).join(' ').toLowerCase()}`;
  const hayWords = words(haystack);
  let total = 0;
  for (const w of words(q)) {
    if (hayWords.some((h) => h.startsWith(w))) total += 3;
    else if (haystack.includes(w)) total += 1;
    else return null;
  }
  if (label.startsWith(q)) total += 5;
  return total + (entry.weight ?? 0);
}

/** Entries matching the query, best first, at most `limit`. With nothing
 *  typed, the entries marked `idle` in the order given. */
export function rank(entries, query, limit = 40) {
  if (!query.trim()) return entries.filter((e) => e.idle).slice(0, limit);
  return entries
    .map((entry, i) => ({ entry, i, s: score(entry, query) }))
    .filter((x) => x.s !== null)
    .sort((a, b) => b.s - a.s || a.entry.label.length - b.entry.label.length || a.i - b.i)
    .slice(0, limit)
    .map((x) => x.entry);
}
