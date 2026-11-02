import { tr } from './i18n.js';
// How a snapshot is described in the list: when, and why it exists.
//
// The list used to title each one with its ID — snap_1790235804433 — which is
// a timestamp in milliseconds and tells a person nothing. The time is the
// name people use ("the one from last night"), and the reason matters as much:
// a copy OpenSave kept just before replacing your save is the one you want
// after a sync went wrong, and it looked exactly like every other row.

/** A snapshot ID's own time: IDs are "snap_" and the creation time in ms. */
export function snapIdTime(id) {
  const m = /^snap_(\d{12,})$/.exec(id ?? '');
  return m ? new Date(Number(m[1])) : null;
}

const timeFmt = { hour: 'numeric', minute: '2-digit' };
const dateTimeFmt = { day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit' };

/** "4:12 PM" for today, "24 Sep, 4:12 PM" otherwise. */
export function whenLabel(date, now = new Date()) {
  const d = date instanceof Date ? date : new Date(date);
  if (Number.isNaN(d.getTime())) return String(date);
  return sameDay(d, now) ? d.toLocaleTimeString(undefined, timeFmt) : d.toLocaleString(undefined, dateTimeFmt);
}

function sameDay(a, b) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

/** "Today", "Yesterday", or the date. */
export function dayLabel(date, now = new Date()) {
  const d = date instanceof Date ? date : new Date(date);
  if (sameDay(d, now)) return tr('snapshots.today');
  const y = new Date(now);
  y.setDate(y.getDate() - 1);
  if (sameDay(d, y)) return tr('snapshots.yesterday');
  return d.toLocaleDateString(undefined, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    ...(d.getFullYear() === now.getFullYear() ? {} : { year: 'numeric' })
  });
}

/** Swaps any snapshot IDs in a comment for the time they name. */
export function humanizeComment(comment, now = new Date()) {
  return (comment ?? '').replace(/snap_\d{12,}/g, (id) => {
    const t = snapIdTime(id);
    return t ? tr('snapshots.namedTime', { time: whenLabel(t, now) }) : id;
  });
}

// Comments the daemon writes when it keeps a copy before changing the save.
// Matched loosely on purpose: the wording has changed between versions and
// older snapshots keep whatever they were given.
const safetyPattern = /^(before|pre-rollback|safety|auto backup before|this device's version)/i;

// The daemon's own wording for the copies it keeps, said plainly. Anything
// not listed is shown as written.
const plainTitles = [
  [/^pre-rollback safety restore point \(before restoring (.+)\)$/i, (m) => tr('snapshots.beforeRestoring', { p0: m[1] })],
  [/^before sync replaced local files$/i, () => tr('snapshots.beforeASyncReplacedIt')],
  [/^safety snapshot before restoring file "(.+)" from (.+)$/i, (m) => tr('snapshots.beforePuttingBackFrom', { p0: m[1], p1: m[2] })],
  [/^auto backup before switching to branch "(.+)"$/i, (m) => tr('snapshots.beforeSwitchingTo', { p0: m[1] })],
  [/^branch "(.+)" created from "(.+)"$/i, (m) => tr('snapshots.startOfCopiedFrom', { p0: m[1], p1: m[2] })]
];

function plainTitle(comment) {
  for (const [re, say] of plainTitles) {
    const m = re.exec(comment);
    if (m) return say(m);
  }
  return capitalise(comment);
}

/**
 * Why a snapshot exists.
 * @returns {{kind: 'manual'|'auto'|'start'|'safety'|'session'|'other', label: string, title: string}}
 *   `label` is the short tag; `title` the line the row leads with.
 */
export function snapshotKind(snap, now = new Date()) {
  let comment = humanizeComment(snap.comment ?? '', now).trim();
  // The daemon stores these when no comment was given; they are defaults,
  // not reasons, and read as noise on every row.
  if (/^(auto backup|manual snapshot)$/i.test(comment)) comment = '';
  if (!snap.isSystemAuto) {
    return { kind: 'manual', label: tr('snapshots.takenByYou'), title: comment || tr('palette.kind.snapshot') };
  }
  if (comment === '') return { kind: 'auto', label: tr('snapshots.automatic'), title: tr('snapshots.saveChanged') };
  if (/^initial snapshot$/i.test(comment)) return { kind: 'start', label: tr('snapshots.automatic'), title: tr('snapshots.whenTrackingStarted') };
  // The save as a play session left it (daemon/sessions.go).
  if (/^after playing/i.test(comment)) return { kind: 'session', label: tr('snapshots.afterPlaying'), title: comment };
  if (safetyPattern.test(comment)) {
    return { kind: 'safety', label: tr('snapshots.safetyCopy'), title: plainTitle(comment) };
  }
  return { kind: 'other', label: tr('snapshots.automatic'), title: plainTitle(comment) };
}

const capitalise = (s) => (s ? s[0].toUpperCase() + s.slice(1) : s);

/** Snapshots, newest first, in runs by day: [{day, snaps}]. */
export function groupByDay(snaps, now = new Date()) {
  const groups = [];
  for (const s of snaps) {
    const day = dayLabel(new Date(s.timestamp), now);
    const last = groups[groups.length - 1];
    if (last && last.day === day) last.snaps.push(s);
    else groups.push({ day, snaps: [s] });
  }
  return groups;
}
