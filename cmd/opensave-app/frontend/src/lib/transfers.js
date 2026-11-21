// The transfers panel: what is moving between this device and others, and
// what moved recently. The daemon keeps the record (internal/transfers); this
// is how it reads.
import { fmtSize } from './format.js';

/** "1.2 MB/s", or '' when there is no speed to show. */
export function speedLabel(bytesPerSec) {
  if (!bytesPerSec || bytesPerSec <= 0) return '';
  return `${fmtSize(bytesPerSec)}/s`;
}

/** "400 KB of 1.0 MB", "1.0 MB", or '' when nothing is known. `ofTotal`
 *  words the running case, for the reader's language. */
export function amountLabel(t, ofTotal = (done, total) => `${done} of ${total}`) {
  if (t.totalBytes > 0 && t.state === 'running') return ofTotal(fmtSize(t.bytesTransferred ?? 0), fmtSize(t.totalBytes));
  if (t.totalBytes > 0) return fmtSize(t.totalBytes);
  if (t.bytesTransferred > 0) return fmtSize(t.bytesTransferred);
  return '';
}

/** The transfers as the panel lists them, with each game's name. */
export function withNames(snapshot, games) {
  const name = (id) => games?.[id]?.name ?? id;
  return {
    active: (snapshot?.active ?? []).map((t) => ({ ...t, name: name(t.gameId) })),
    recent: (snapshot?.recent ?? []).map((t) => ({ ...t, name: name(t.gameId) }))
  };
}
