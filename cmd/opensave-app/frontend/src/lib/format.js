// Small formatters shared by the views.

/** Bytes as KB or MB, one decimal: "3.2 KB", "1.4 MB". */
export const fmtSize = (n) =>
  n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : (n / 1024).toFixed(1) + ' KB';

/** A timestamp in the viewer's locale, or a dash for none. */
export const fmtTime = (t) => (t ? new Date(t).toLocaleString() : '—');

/** "1 game", "3 games". */
export const plural = (n, word, many = word + 's') => `${n} ${n === 1 ? word : many}`;
