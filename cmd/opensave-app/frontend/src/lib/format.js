// Small formatters shared by the views.

/** Bytes as B, KB, MB or GB, one decimal past bytes: "185 B", "3.2 KB",
 *  "1.4 MB", "9.5 GB". GB so free space on a drive reads as "9.5 GB" and not
 *  "9724.7 MB"; bytes so a small save file is not "0.0 KB". */
export const fmtSize = (n) =>
  (n ?? 0) < 1024
    ? `${Math.max(0, Math.round(n ?? 0))} B`
    : n >= 1073741824
    ? (n / 1073741824).toFixed(1) + ' GB'
    : n >= 1048576
      ? (n / 1048576).toFixed(1) + ' MB'
      : (n / 1024).toFixed(1) + ' KB';

/** A timestamp in the viewer's locale, or a dash for none. */
export const fmtTime = (t) => (t ? new Date(t).toLocaleString() : '—');

/** "1 game", "3 games". */
export const plural = (n, word, many = word + 's') => `${n} ${n === 1 ? word : many}`;

/** How long something was played: "45 min", "3 h", "14 h 20 min". */
export function playLength(ms) {
  const minutes = Math.round((ms ?? 0) / 60000);
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return `${m} min`;
  return m ? `${h} h ${m} min` : `${h} h`;
}
