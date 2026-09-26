// The activity timeline (GET /api/activity, internal/daemon/activity.go):
// what happened to every game's save — syncs with other devices, snapshots,
// play sessions, restores — said as a person would say it.
import { snapshotKind, whenLabel } from './snapshots.js';
import { fmtSize, playLength, plural } from './format.js';

/** The filters over the timeline, and the kinds each shows. */
// A conflict and an emptied save are things that happened to a sync, so they
// are among the syncs; a filter of their own was nearly always empty, and
// what is waiting on you now is the bell's to say.
export const TIMELINE_FILTERS = {
  all: { label: 'Everything', kinds: null },
  syncs: { label: 'Syncs', kinds: ['received', 'sent', 'deleted', 'cloud-pulled', 'conflict', 'emptied'] },
  snapshots: { label: 'Snapshots', kinds: ['snapshot', 'restored'] },
  play: { label: 'Play', kinds: ['played'] }
};

export function filterItems(items, filter = 'all', gameId = '') {
  const kinds = TIMELINE_FILTERS[filter]?.kinds;
  return items.filter((it) => (!kinds || kinds.includes(it.kind)) && (!gameId || it.gameId === gameId));
}

/**
 * One timeline item in words.
 * @returns {{icon: string, title: string, detail: string, tone: 'ok'|'warn'|'muted'}}
 */
export function describe(item, now = new Date()) {
  const files = item.files ? plural(item.files, 'file') : 'files';
  const where = item.detail && item.kind !== 'restored' ? ` ${item.detail}` : '';
  switch (item.kind) {
    case 'received':
      return { icon: 'down', title: `Got ${files} from ${item.device}${where}`, detail: item.bytes ? fmtSize(item.bytes) : '', tone: 'ok' };
    case 'sent':
      return { icon: 'up', title: `${item.device} took ${files} from here`, detail: '', tone: 'ok' };
    case 'deleted':
      return { icon: 'trash', title: `Deleted ${files} as ${item.device} did${where}`, detail: '', tone: 'muted' };
    case 'restored': {
      const [, at] = (item.detail ?? '').split('|');
      return { icon: 'restore', title: at ? `Put back the snapshot of ${whenLabel(at, now)}` : 'Put back a snapshot', detail: '', tone: 'ok' };
    }
    case 'cloud-pulled':
      return { icon: 'cloud', title: `Brought ${item.device}'s newer save from the cloud`, detail: '', tone: 'ok' };
    case 'emptied':
      return { icon: 'alert', title: 'Every save file was deleted here', detail: item.files ? `${files} kept on your other devices until you chose` : '', tone: 'warn' };
    case 'conflict':
      return { icon: 'alert', title: `Changed here and on ${item.device} at once`, detail: 'Needed a decision', tone: 'warn' };
    case 'played':
      return { icon: 'play', title: item.durationMs ? `Played for ${playLength(item.durationMs)}` : 'Played', detail: 'On this device', tone: 'ok' };
    case 'snapshot': {
      if (item.device) return { icon: 'camera', title: `Kept a copy of ${item.device}'s save`, detail: '', tone: 'muted' };
      const why = snapshotKind({ comment: item.comment, isSystemAuto: item.auto }, now);
      return { icon: 'camera', title: why.kind === 'manual' ? `Snapshot: ${why.title}` : why.title, detail: why.kind === 'manual' ? 'Taken by you' : '', tone: 'muted' };
    }
    default:
      return { icon: 'dot', title: item.kind, detail: '', tone: 'muted' };
  }
}

/**
 * The timeline as runs: the same thing happening to the same game again and
 * again — a save changing every few minutes while it is played — is one line
 * with a count and the time of the newest.
 */
export function runs(items, now = new Date()) {
  const out = [];
  for (const item of items) {
    const said = describe(item, now);
    const last = out[out.length - 1];
    if (last && last.item.gameId === item.gameId && last.item.kind === item.kind && last.said.title === said.title && item.kind !== 'played') {
      last.count++;
      continue;
    }
    out.push({ item, said, count: 1 });
  }
  return out;
}

/** Runs in days, newest first: [{day, runs}]. */
export function byDay(list, now = new Date()) {
  const days = [];
  for (const run of list) {
    const d = new Date(run.item.atMs);
    const key = d.toDateString();
    let day = days[days.length - 1];
    if (!day || day.key !== key) {
      day = { key, day: dayName(d, now), runs: [] };
      days.push(day);
    }
    day.runs.push(run);
  }
  return days;
}

function dayName(d, now) {
  const start = (x) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diff = Math.round((start(now) - start(d)) / 86_400_000);
  if (diff === 0) return 'Today';
  if (diff === 1) return 'Yesterday';
  return d.toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long', ...(d.getFullYear() === now.getFullYear() ? {} : { year: 'numeric' }) });
}

/** Where a game was last played, in words: this device, or another. */
export function playedWhere(g, thisDevice) {
  if (!g?.lastPlayedAt) return '';
  return g.lastPlayedOn ? g.lastPlayedOn : thisDevice ? `${thisDevice} (here)` : 'Here';
}
