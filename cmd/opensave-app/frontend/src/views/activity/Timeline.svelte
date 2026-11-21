<script>
  // Everything that happened to the saves, game by game: where each game was
  // last played and when it was last snapshotted and synced, and then the
  // timeline itself — syncs with your other devices, snapshots, play
  // sessions, restores. See lib/timeline.js and internal/daemon/activity.go.
  import { onDestroy } from 'svelte';
  import ArrowDownToLine from 'lucide-svelte/icons/arrow-down-to-line';
  import ArrowUpFromLine from 'lucide-svelte/icons/arrow-up-from-line';
  import Trash2 from 'lucide-svelte/icons/trash-2';
  import RotateCcw from 'lucide-svelte/icons/rotate-ccw';
  import CloudDownload from 'lucide-svelte/icons/cloud-download';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import Camera from 'lucide-svelte/icons/camera';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Dot from 'lucide-svelte/icons/dot';
  import CoverImage from '../../components/CoverImage.svelte';
  import FilterTabs from '../../components/ui/FilterTabs.svelte';
  import { api, gameCover } from '../../lib/api.js';
  import { games, navigate, activityTick } from '../../lib/stores.js';
  import { t } from '../../lib/i18n.js';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { playLength, fmtSize } from '../../lib/format.js';
  import { snapshotKind, whenLabel } from '../../lib/snapshots.js';
  import { TIMELINE_FILTERS, byDay, filterItems, runs } from '../../lib/timeline.js';
  import { gatedFlip } from '../../lib/motion.js';

  // Each entry keeps its key while the list changes around it, so one that
  // arrives comes in at the top and the rest move down to make room, rather
  // than every row taking the next one's words. Two entries alike in every
  // way are told apart by their order.
  function keyed(list) {
    const seen = new Map();
    return list.map((run) => {
      const base = `${run.item.gameId}|${run.item.kind}|${run.item.atMs}|${run.count}`;
      const n = seen.get(base) ?? 0;
      seen.set(base, n + 1);
      return { run, key: `${base}|${n}` };
    });
  }

  // timeAgo's facts as a catalog key and its params, ready for $t.
  const agoKey = (iso, now = Date.now()) => {
    const p = timeAgoParts(iso, now);
    if (p.kind === 'invalid') return { text: p.text };
    const { kind, ...params } = p;
    return { key: 'timeAgo.' + kind, params };
  };

  // The timeline's wording is built in lib/timeline.js's describe(), in
  // English; the same cases, in the same order, are mapped here to catalog
  // keys and params (see locales/en.json, "activity.event."), with $t for
  // the pieces the lib alone knows the words to. icon and tone stay said's.
  function eventSay(item, now, t) {
    const loc = item.detail && item.kind !== 'restored' ? /^in its "(.+)" location$/.exec(item.detail) : null;
    const where = !item.detail || item.kind === 'restored' ? '' : loc ? t('activity.event.inLocation', { root: loc[1] }) : ` ${item.detail}`;
    const device = item.device ?? '';
    switch (item.kind) {
      case 'received':
        return {
          title: item.files
            ? { key: 'activity.event.received', params: { n: item.files, device, where } }
            : { key: 'activity.event.receivedAny', params: { device, where } },
          detail: item.bytes ? { text: fmtSize(item.bytes) } : null
        };
      case 'sent':
        return {
          title: item.files
            ? { key: 'activity.event.sent', params: { n: item.files, device, where } }
            : { key: 'activity.event.sentAny', params: { device, where } },
          detail: null
        };
      case 'deleted':
        return {
          title: item.files
            ? { key: 'activity.event.deleted', params: { n: item.files, device, where } }
            : { key: 'activity.event.deletedAny', params: { device, where } },
          detail: null
        };
      case 'restored': {
        const at = (item.detail ?? '').split('|')[1];
        return { title: at ? { key: 'activity.event.restoredWhen', params: { when: whenLabel(at, now) } } : { key: 'activity.event.restored', params: {} }, detail: null };
      }
      case 'cloud-pulled':
        return { title: { key: 'activity.event.cloudPulled', params: { device } }, detail: null };
      case 'emptied':
        return {
          title: { key: 'activity.event.emptied', params: {} },
          detail: item.files ? { key: 'activity.event.keptElsewhere', params: { n: item.files } } : null
        };
      case 'conflict':
        return { title: { key: 'activity.event.conflict', params: { device, where } }, detail: { key: 'activity.event.neededDecision', params: {} } };
      case 'played':
        return {
          title: item.durationMs ? { key: 'activity.event.playedFor', params: { length: playLength(item.durationMs) } } : { key: 'activity.event.played', params: {} },
          detail: device ? { key: 'activity.event.onDevice', params: { device } } : { key: 'activity.event.onThisDevice', params: {} }
        };
      case 'snapshot': {
        if (item.device) return { title: { key: 'activity.event.keptCopy', params: { device } }, detail: null };
        const why = snapshotKind({ comment: item.comment, isSystemAuto: item.auto }, now);
        if (why.kind === 'manual')
          return {
            title: { key: 'activity.event.snapshotManual', params: { comment: why.title } },
            detail: { key: 'activity.event.takenByYou', params: {} }
          };
        if (why.kind === 'auto') return { title: { key: 'activity.event.saveChanged', params: {} }, detail: null };
        if (why.kind === 'start') return { title: { key: 'activity.event.trackingStart', params: {} }, detail: null };
        // A safety or other kind's title is the daemon's own comment, which
        // only exists in English: shown as it came.
        return { title: { text: why.title }, detail: null };
      }
      default:
        return { title: { text: item.kind }, detail: null };
    }
  }

  const ICONS = {
    down: ArrowDownToLine,
    up: ArrowUpFromLine,
    trash: Trash2,
    restore: RotateCcw,
    cloud: CloudDownload,
    alert: TriangleAlert,
    camera: Camera,
    play: Gamepad2,
    dot: Dot
  };
  const initials = (name = '') => name.split(/\s+/).map((w) => w[0]).join('').slice(0, 2).toUpperCase();
  // A missing stamp (a game never played or synced) must fall through to
  // the catalog's "never" rather than throw on toISOString().
  const iso = (ms) => (ms == null || ms === '' ? null : new Date(ms).toISOString());

  /** One device's name, or '' for every device (the page's filter). */
  export let device = '';

  let report = null;
  let error = '';
  let loadingMore = false;
  let filter = 'all';
  let gameId = '';

  async function load() {
    try {
      report = await api.get('/api/activity?limit=150');
      error = '';
    } catch (e) {
      error = e.message;
    }
  }
  async function loadMore() {
    const oldest = report.items[report.items.length - 1]?.atMs;
    if (!oldest) return;
    loadingMore = true;
    try {
      const older = await api.get(`/api/activity?limit=150&before=${oldest}`);
      report = { ...report, items: [...report.items, ...older.items], more: older.more };
    } catch (e) {
      error = e.message;
    } finally {
      loadingMore = false;
    }
  }
  load();

  // New activity comes as it happens; gathered for a moment, since a sync
  // can bring several at once.
  let pending = null;
  const stop = activityTick.subscribe((n) => {
    if (!n) return;
    clearTimeout(pending);
    pending = setTimeout(load, 700);
  });
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 60_000);
  onDestroy(() => {
    stop();
    clearTimeout(pending);
    clearInterval(tick);
  });

  // One device's events, or every device's: an event with no device of its
  // own happened here.
  $: here = report?.device ?? '';
  $: shown = filterItems(
    (report?.items ?? []).filter((it) => $games[it.gameId] && (!device || (it.device || here) === device)),
    filter,
    gameId
  );
  $: days = byDay(runs(shown, new Date(now)), new Date(now));
</script>

{#if error}
  <div class="card"><p class="hint">{$t('activity.timeline.readError', { error })}</p></div>
{:else if !report}
  <div class="skeleton" style="height: 220px"></div>
{:else}
  <div class="filters">
    <FilterTabs options={Object.entries(TIMELINE_FILTERS).map(([id]) => [id, $t('activity.filter.' + id)])} bind:value={filter} />
    <select bind:value={gameId} aria-label={$t('activity.timeline.showOneGameAria')}>
      <option value="">{$t('activity.timeline.everyGame')}</option>
      {#each Object.values($games).sort((a, b) => a.name.localeCompare(b.name)) as game (game.id)}
        <option value={game.id}>{game.name}</option>
      {/each}
    </select>
  </div>

  {#if days.length === 0}
    <div class="card"><div class="empty"><h3>{$t('activity.timeline.emptyTitle')}</h3><p>{$t('activity.timeline.emptyBody')}</p></div></div>
  {:else}
    {#each days as day (day.key)}
      <h4 class="day">{day.day === 'Today' ? $t('activity.day.today') : day.day === 'Yesterday' ? $t('activity.day.yesterday') : day.day}</h4>
      <div class="card entries">
        {#each keyed(day.runs) as { run, key }, i (key)}
          {@const game = $games[run.item.gameId]}
          {@const said = eventSay(run.item, new Date(now), $t)}
          <button class="entry tone-{run.said.tone}" style="--i: {Math.min(i, 10)}" animate:gatedFlip on:click={() => navigate('game', { gameId: run.item.gameId })}>
            <span class="kind"><svelte:component this={ICONS[run.said.icon] ?? Dot} size={15} /></span>
            <span class="thumb"><span class="initials">{initials(game.name)}</span><CoverImage src={gameCover(game)} alt="" /></span>
            <span class="what">
              <span class="line"><span class="gname">{game.name}</span> {said.title.text ?? $t(said.title.key, said.title.params)}{#if run.count > 1}<span class="count">×{run.count}</span>{/if}</span>
              {#if said.detail}<span class="sub">{said.detail.text ?? $t(said.detail.key, said.detail.params)}</span>{/if}
            </span>
            <span class="clock" title={new Date(run.item.atMs).toLocaleString()}>
              {new Date(run.item.atMs).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}
            </span>
          </button>
        {/each}
      </div>
    {/each}
    {#if report.more && filter === 'all' && !gameId}
      <div class="more">
        <button class="btn small" disabled={loadingMore} on:click={loadMore}>{loadingMore ? $t('activity.timeline.loading') : $t('activity.timeline.showOlder')}</button>
      </div>
    {/if}
  {/if}
{/if}

<style>
  .sub {
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  .thumb {
    position: relative;
    flex: none;
    width: 30px;
    height: 30px;
    border-radius: 6px;
    overflow: hidden;
    background: var(--bg-hover);
    display: grid;
    place-items: center;
  }
  .thumb :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .initials {
    font-size: 0.66rem;
    font-weight: 700;
    color: var(--text-faint);
  }
  .filters {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
    margin-bottom: 10px;
  }
  select {
    padding: 6px 10px;
    background-color: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.85rem;
  }
  .day {
    margin: 16px 0 8px;
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--text-faint);
  }
  .entries {
    padding: 4px;
  }
  .entry {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 7px 8px;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .entry:hover {
    background: var(--bg-hover);
  }
  .kind {
    flex: none;
    width: 18px;
    display: grid;
    place-items: center;
    color: var(--text-faint);
  }
  .tone-warn .kind {
    color: var(--warn);
  }
  .tone-ok .kind {
    color: var(--text-dim);
  }
  .what {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .line {
    font-size: 0.86rem;
    color: var(--text-dim);
  }
  .gname {
    font-weight: 600;
    color: var(--text);
  }
  .count {
    margin-left: 6px;
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  .clock {
    flex: none;
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  .more {
    display: flex;
    justify-content: center;
    margin: 14px 0;
  }
  .empty {
    text-align: center;
    padding: 28px 12px;
  }
  .empty p {
    color: var(--text-faint);
    font-size: 0.86rem;
  }
  /* In, one after another down the day: a short rise, movement only. One
     that arrives later comes in the same way, at the top. */
  .entry {
    animation: entry-in 0.4s cubic-bezier(0.2, 0.8, 0.2, 1) backwards;
    animation-delay: calc(var(--i, 0) * 28ms);
  }
  @keyframes entry-in {
    from {
      transform: translateY(8px);
    }
  }
</style>
