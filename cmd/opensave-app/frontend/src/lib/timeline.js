import { tr } from './i18n.js';
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
  all: { get label() { return tr('timeline.everything'); }, kinds: null },
  syncs: { get label() { return tr('timeline.syncs'); }, kinds: ['received', 'sent', 'deleted', 'cloud-pulled', 'conflict', 'emptied'] },
  snapshots: { get label() { return tr('gameDetail.snapshots'); }, kinds: ['snapshot', 'restored'] },
  play: { get label() { return tr('timeline.play'); }, kinds: ['played'] }
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
      return { icon: 'down', title: tr('timeline.gotFrom', { p0: files, p1: item.device, p2: where }), detail: item.bytes ? fmtSize(item.bytes) : '', tone: 'ok' };
    case 'sent':
      return { icon: 'up', title: tr('timeline.tookFromHere', { p0: item.device, p1: files }), detail: '', tone: 'ok' };
    case 'deleted':
      return { icon: 'trash', title: tr('timeline.deletedAsDid', { p0: files, p1: item.device, p2: where }), detail: '', tone: 'muted' };
    case 'restored': {
      const [, at] = (item.detail ?? '').split('|');
      return { icon: 'restore', title: at ? tr('timeline.putBackTheSnapshotOf', { p0: whenLabel(at, now) }) : tr('timeline.putBackASnapshot'), detail: '', tone: 'ok' };
    }
    case 'cloud-pulled':
      return { icon: 'cloud', title: tr('stores.broughtSNewerSaveFromTheCloud', { p0: item.device }), detail: '', tone: 'ok' };
    case 'emptied':
      return { icon: 'alert', title: tr('timeline.everySaveFileWasDeletedHere'), detail: item.files ? tr('timeline.keptOnYourOtherDevicesUntil', { p0: files }) : '', tone: 'warn' };
    case 'conflict':
      return { icon: 'alert', title: tr('timeline.changedHereAndOnAtOnce', { p0: item.device }), detail: tr('timeline.neededADecision'), tone: 'warn' };
    case 'played':
      return { icon: 'play', title: item.durationMs ? tr('timeline.playedFor', { p0: playLength(item.durationMs) }) : tr('timeline.played'), detail: tr('linkPicker.onThisDevice'), tone: 'ok' };
    case 'snapshot': {
      if (item.device) return { icon: 'camera', title: tr('timeline.keptACopyOfSSave', { p0: item.device }), detail: '', tone: 'muted' };
      const why = snapshotKind({ comment: item.comment, isSystemAuto: item.auto }, now);
      return { icon: 'camera', title: why.kind === 'manual' ? tr('timeline.snapshot', { p0: why.title }) : why.title, detail: why.kind === 'manual' ? tr('snapshots.takenByYou') : '', tone: 'muted' };
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
  if (diff === 0) return tr('snapshots.today');
  if (diff === 1) return tr('snapshots.yesterday');
  return d.toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long', ...(d.getFullYear() === now.getFullYear() ? {} : { year: 'numeric' }) });
}

/** Where a game was last played, in words: this device, or another. */
export function playedWhere(g, thisDevice) {
  if (!g?.lastPlayedAt) return '';
  return g.lastPlayedOn ? g.lastPlayedOn : thisDevice ? tr('timeline.here', { p0: thisDevice }) : tr('timeline.hereLabel');
}
