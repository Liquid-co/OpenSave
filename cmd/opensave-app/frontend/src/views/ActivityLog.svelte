<script>
  // Activity: how the games have been played and kept, on every device.
  // Along the top, a few figures for the period; then three views — your
  // games as a table (lib/activityview.js), the events as they happened
  // (activity/Timeline.svelte), and the detailed log, every line the app
  // wrote, for when something needs looking into.
  import { slidingIndicator, rise } from '../lib/motion.js';
  import { onDestroy } from 'svelte';
  import Check from 'lucide-svelte/icons/check';
  import Dot from 'lucide-svelte/icons/dot';
  import Exclamation from 'lucide-svelte/icons/triangle-alert';
  import X from 'lucide-svelte/icons/x';
  import ChevronDown from 'lucide-svelte/icons/chevron-down';
  import MonitorSmartphone from 'lucide-svelte/icons/monitor-smartphone';
  import CalendarRange from 'lucide-svelte/icons/calendar-range';
  import { logEntries, gameList, navigate, toast, peers, activityTick, games } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import { asText, feedByDay, filterEntries, linkGames } from '../lib/activity.js';
  import { PERIODS, periodStart, deviceSlots, activityStats, barParts } from '../lib/activityview.js';
  import { playLength } from '../lib/format.js';
  import { t as i18n } from '../lib/i18n.js';
  import SearchInput from '../components/ui/SearchInput.svelte';
  import FilterTabs from '../components/ui/FilterTabs.svelte';
  import Dropdown from '../components/ui/Dropdown.svelte';
  import Timeline from './activity/Timeline.svelte';
  import YourGames from './activity/YourGames.svelte';

  // Which view, remembered on this device. "timeline" was the name before
  // the games had a view of their own; it meant them first.
  const VIEW_KEY = 'opensave.activityView';
  const VIEWS = ['games', 'events', 'log'];
  let view = 'games';
  try {
    const saved = localStorage.getItem(VIEW_KEY);
    view = VIEWS.includes(saved) ? saved : 'games';
  } catch {}
  $: try {
    localStorage.setItem(VIEW_KEY, view);
  } catch {}

  // ── The period and the device ──
  let period = 'month';
  let device = '';
  let periodOpen = false;
  let deviceOpen = false;
  let periodAnchor;
  let deviceAnchor;
  const outside = (e) => {
    if (periodOpen && !periodAnchor?.contains(e.target)) periodOpen = false;
    if (deviceOpen && !deviceAnchor?.contains(e.target)) deviceOpen = false;
  };

  // ── The report, reaching back to the start of the period ──
  // A page at a time, up to ten (5,000 things): past that the counts say
  // "at least".
  let report = null;
  let reportError = '';
  let loading = 0;
  async function loadReport(p) {
    const ticket = ++loading;
    try {
      const from = periodStart(p);
      let r = await api.get('/api/activity?limit=500');
      for (let page = 1; r.more && page < 10; page++) {
        const oldest = r.items[r.items.length - 1]?.atMs;
        if (!oldest || oldest < from) break;
        const older = await api.get(`/api/activity?limit=500&before=${oldest}`);
        r = { ...r, items: [...r.items, ...older.items], more: older.more };
      }
      if (ticket !== loading) return;
      report = r;
      reportError = '';
    } catch (e) {
      if (ticket === loading) reportError = e.message;
    }
  }
  $: loadReport(period);
  let pending = null;
  const stopTick = activityTick.subscribe((n) => {
    if (!n) return;
    clearTimeout(pending);
    pending = setTimeout(() => loadReport(period), 800);
  });

  $: here = report?.device ?? '';
  $: reportDevices = (report?.games ?? []).flatMap((g) => (g.devices ?? []).map((d) => d.device));
  $: slots = deviceSlots(here, Object.values($peers), reportDevices);
  $: deviceNames = [...slots.keys()];
  $: if (device && !deviceNames.includes(device)) device = '';
  $: complete = !!report && (!report.more || (report.items[report.items.length - 1]?.atMs ?? 0) < periodStart(period));
  $: stats = activityStats({ items: report?.items ?? [], games: (report?.games ?? []).filter((g) => $games[g.gameId]), here, period, device, complete });
  $: playedParts = barParts(stats.byDevice, slots);
  $: atLeast = (n) => (stats.complete ? String(n) : `${n}+`);

  $: periodItems = PERIODS.map((id) => ({ label: $i18n('activity.period.' + id), checked: period === id, run: () => (period = id) }));
  $: deviceItems = [
    { label: $i18n('activity.devicesAll'), checked: device === '', run: () => (device = '') },
    null,
    ...deviceNames.map((name) => ({ label: name, checked: device === name, run: () => (device = name) }))
  ];

  // ── The detailed log ──
  // Remembered on this device only; nothing depends on them.
  const MODE_KEY = 'opensave.activityMode';
  let technical = false;
  try {
    technical = localStorage.getItem(MODE_KEY) === 'technical';
  } catch {}
  $: try {
    localStorage.setItem(MODE_KEY, technical ? 'technical' : 'feed');
  } catch {}

  let show = 'highlights';
  let query = '';
  $: problemCount = filterEntries($logEntries, { show: 'problems' }).length;
  $: shown = filterEntries($logEntries, { show, query });
  $: groups = feedByDay(shown, now);

  let now = new Date();
  const tick = setInterval(() => (now = new Date()), 60_000);
  onDestroy(() => {
    clearInterval(tick);
    clearTimeout(pending);
    stopTick();
  });

  const icons = { success: Check, info: Dot, warn: Exclamation, error: X };
  const fmtClock = (t) => new Date(t).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
  const fmtStamp = (t) => new Date(t).toLocaleTimeString();

  async function copyShown() {
    try {
      await navigator.clipboard.writeText(asText(shown));
      toast($i18n('activity.copiedEntries', { n: shown.length }), 'success');
    } catch (e) {
      toast($i18n('activity.copyFailed', { error: e.message }), 'error');
    }
  }

  // The technical log sticks to the bottom as new entries arrive, like a
  // terminal. Implemented as an action (node is a local, uninstrumented
  // reference): assigning scrollTop on a bind:this variable would
  // $$invalidate it and loop the reactive flush forever, freezing the app.
  function autoscroll(node, _count) {
    const toBottom = () => queueMicrotask(() => (node.scrollTop = node.scrollHeight));
    toBottom();
    return { update: toBottom };
  }
</script>

<svelte:window on:mousedown={outside} />

<div class="head">
  <h1 class="page-title">{$i18n('sidebar.nav.activity')}</h1>
  <div class="filters">
    <div class="anchor" bind:this={deviceAnchor}>
      <button class="tool" class:on={deviceOpen} aria-haspopup="menu" aria-expanded={deviceOpen} on:click={() => (deviceOpen = !deviceOpen)}>
        <MonitorSmartphone size={15} />{device || $i18n('activity.devicesAll')}<span class="chev"><ChevronDown size={13} /></span>
      </button>
      <Dropdown items={deviceItems} bind:open={deviceOpen} label={$i18n('activity.deviceLabel')} />
    </div>
    <div class="anchor" bind:this={periodAnchor}>
      <button class="tool" class:on={periodOpen} aria-haspopup="menu" aria-expanded={periodOpen} on:click={() => (periodOpen = !periodOpen)}>
        <CalendarRange size={15} />{$i18n('activity.period.' + period)}<span class="chev"><ChevronDown size={13} /></span>
      </button>
      <Dropdown items={periodItems} bind:open={periodOpen} label={$i18n('activity.periodLabel')} />
    </div>
  </div>
</div>

<section class="stats" aria-label={$i18n('activity.period.' + period)}>
  <div class="stat" style="--i: 0">
    <span class="cap">{$i18n('activity.stat.played')}</span>
    {#key stats.playedMs}<span class="v" in:rise={{ y: 4 }}>{stats.playedMs ? playLength(stats.playedMs) : '—'}</span>{/key}
    <span class="bar" role="img" aria-label={playedParts.map((p) => `${p.device} ${playLength(p.ms)}`).join(', ')}>
      {#each playedParts as p (p.device)}<span class="seg slot-{p.slot}" style="flex: {p.share}" title="{p.device}: {playLength(p.ms)}"></span>{:else}<span class="seg empty"></span>{/each}
    </span>
  </div>
  <div class="stat" style="--i: 1">
    <span class="cap">{$i18n('activity.stat.sessions')}</span>
    {#key stats.sessions}<span class="v" in:rise={{ y: 4 }}>{stats.sessions}</span>{/key}
    <span class="sub">{stats.longest ? $i18n('activity.stat.longest', { length: playLength(stats.longest.ms), name: $games[stats.longest.gameId]?.name ?? '' }) : $i18n('activity.stat.noSessions')}</span>
  </div>
  <div class="stat" style="--i: 2">
    <span class="cap">{$i18n('activity.stat.snapshots')}</span>
    {#key stats.snapshots}<span class="v" in:rise={{ y: 4 }}>{atLeast(stats.snapshots)}</span>{/key}
    <span class="sub">{$i18n('activity.stat.takenByYou', { n: stats.takenByYou })}</span>
  </div>
  <div class="stat" style="--i: 3">
    <span class="cap">{$i18n('activity.stat.synced')}</span>
    {#key stats.syncs}<span class="v" in:rise={{ y: 4 }}>{$i18n('activity.stat.syncTimes', { n: stats.syncs, count: atLeast(stats.syncs) })}</span>{/key}
    <span class="sub">{stats.syncDevices ? $i18n('activity.stat.withDevices', { n: stats.syncDevices }) : $i18n('activity.stat.noSyncs')}</span>
  </div>
</section>

<div class="tabs-row">
  <div class="pills" role="tablist" aria-label={$i18n('activity.viewAria')} use:slidingIndicator={{ mode: 'fill', className: 'pill-mark' }}>
    {#each VIEWS as id}
      <button role="tab" class="tab" aria-selected={view === id} class:on={view === id} on:click={() => (view = id)}>{$i18n('activity.tab.' + id)}</button>
    {/each}
  </div>
  <span class="grow"></span>
  {#if view === 'games' && deviceNames.length > 1}
    <ul class="legend" aria-label={$i18n('activity.legend')}>
      {#each deviceNames as name (name)}
        <li class:dim={device && device !== name}><span class="dot slot-{slots.get(name)}" aria-hidden="true"></span>{name}</li>
      {/each}
    </ul>
  {/if}
  {#if view === 'log'}
    <div class="controls">
      <FilterTabs
        options={[['highlights', $i18n('activity.filterHighlights')], ['all', $i18n('activity.filterAll')], ['problems', $i18n('activity.filterProblems')]]}
        counts={{ problems: problemCount }}
        bind:value={show}
      />
      <SearchInput placeholder={$i18n('activity.searchPlaceholder')} bind:value={query} />
      <label class="toggle" title={$i18n('activity.technicalTitle')}>
        <input type="checkbox" bind:checked={technical} />
        {$i18n('activity.technicalLog')}
      </label>
      <button class="btn small" disabled={shown.length === 0} on:click={copyShown} title={$i18n('activity.copyTitle')}>
        {$i18n('common.copy')}
      </button>
    </div>
  {/if}
</div>

{#if view === 'games'}
  {#if reportError}
    <div class="card"><p class="hint">{$i18n('activity.timeline.readError', { error: reportError })}</p></div>
  {:else if !report}
    <div class="skeleton" style="height: 320px; border-radius: var(--radius-lg)"></div>
  {:else}
    <div in:rise={{ y: 6 }}><YourGames {report} {period} {device} {slots} /></div>
  {/if}
{:else if view === 'events'}
  <div in:rise={{ y: 6 }}><Timeline {device} /></div>
{:else if $logEntries.length === 0}
  <div class="card"><div class="empty"><h3>{$i18n('activity.emptyTitle')}</h3><p>{$i18n('activity.emptyBody')}</p></div></div>
{:else if shown.length === 0}
  <div class="card"><div class="empty"><h3>{$i18n('activity.noMatchTitle')}</h3><p>
        {#if query}{$i18n('activity.tryAnotherSearch')}{:else if show === 'problems'}{$i18n('activity.allQuiet')}{:else}{$i18n('activity.routineBefore')}<button class="linklike" on:click={() => (show = 'all')}>{$i18n('activity.routineLink')}</button>{$i18n('activity.routineAfter')}{/if}
      </p></div></div>
{:else if technical}
  <div class="card log" use:autoscroll={shown.length}>
    {#each shown as entry}
      <div class="line">
        <span class="stamp">{fmtStamp(entry.timestamp)}</span>
        <span class="level lvl-{entry.level}">{entry.level}</span>
        <span class="raw">{entry.message}</span>
      </div>
    {/each}
  </div>
{:else}
  <div class="feed">
    {#each groups as group (group.day)}
      <h4 class="day">{group.day}</h4>
      <div class="card entries">
        {#each group.entries as entry}
          <div class="entry lvl-{entry.level}">
            <span class="icon" aria-label={entry.level}><svelte:component this={icons[entry.level] ?? Dot} size={entry.level === 'info' ? 18 : 11} strokeWidth={entry.level === 'info' ? 3 : 3.2} /></span>
            <span class="msg">
              {#each linkGames(entry.message, $gameList) as part}
                {#if part.gameId}
                  <button class="game" on:click={() => navigate('game', { gameId: part.gameId })}>{part.text}</button>
                {:else}{part.text}{/if}
              {/each}
            </span>
            <span class="clock" title={new Date(entry.timestamp).toLocaleString()}>{fmtClock(entry.timestamp)}</span>
          </div>
        {/each}
      </div>
    {/each}
  </div>
{/if}

<style>
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
    margin-bottom: 20px;
  }
  .page-title {
    font-size: var(--text-2xl);
    font-weight: 600;
    letter-spacing: -0.01em;
  }
  .filters {
    display: flex;
    gap: 6px;
  }
  .anchor {
    position: relative;
  }
  .tool {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 34px;
    padding: 0 12px;
    border: 1px solid var(--border);
    border-radius: 9px;
    background: var(--bg-raised);
    color: var(--text);
    font: inherit;
    font-size: var(--text-md);
    font-weight: 500;
    white-space: nowrap;
    cursor: pointer;
    transition:
      background 0.12s,
      border-color 0.12s;
  }
  .tool:hover,
  .tool.on {
    background: var(--bg-hover);
    border-color: var(--border-strong);
  }
  .chev {
    display: inline-flex;
    color: var(--text-faint);
    transition: transform 0.18s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .tool.on .chev {
    transform: rotate(180deg);
  }

  /* The period's figures. */
  .stats {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 12px;
    margin-bottom: 22px;
  }
  .stat {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 14px 16px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    min-width: 0;
    animation: stat-in 0.3s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 50ms);
  }
  @keyframes stat-in {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }
  .cap {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .v {
    display: inline-block;
    font-size: var(--text-xl);
    font-weight: 650;
    letter-spacing: -0.01em;
    font-variant-numeric: tabular-nums;
  }
  .sub {
    font-size: var(--text-sm);
    color: var(--text-dim);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .bar {
    display: flex;
    gap: 2px;
    height: 6px;
    margin-top: 6px;
  }
  .seg {
    border-radius: 3px;
    min-width: 4px;
    transition: flex 0.4s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .seg.empty {
    flex: 1;
    background: var(--bg-active);
  }

  /* The views, and which colour is which device. */
  .tabs-row {
    display: flex;
    align-items: center;
    gap: 10px 14px;
    flex-wrap: wrap;
    margin-bottom: 14px;
  }
  .grow {
    flex: 1;
  }
  .pills {
    display: flex;
    gap: 4px;
  }
  .tab {
    height: 32px;
    padding: 0 14px;
    border: 1px solid transparent;
    border-radius: 999px;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-base);
    cursor: pointer;
    transition: color 0.12s;
  }
  .tab:hover {
    color: var(--text);
  }
  .tab.on {
    color: var(--text);
    font-weight: 600;
    background: var(--bg-active);
    border-color: var(--border-strong);
  }
  .pills:global(.slides) .tab {
    position: relative;
    z-index: 1;
  }
  .pills:global(.slides) .tab.on {
    background: transparent;
    border-color: transparent;
  }
  .pills :global(.pill-mark) {
    background: var(--bg-active);
    border: 1px solid var(--border-strong);
    border-radius: 999px;
  }
  .legend {
    list-style: none;
    display: flex;
    gap: 14px;
    flex-wrap: wrap;
    font-size: var(--text-sm);
    color: var(--text-dim);
  }
  .legend li {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    transition: opacity 0.15s;
  }
  .legend li.dim {
    opacity: 0.4;
  }
  .dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
  }
  .slot-0 {
    background: var(--device-0);
  }
  .slot-1 {
    background: var(--device-1);
  }
  .slot-2 {
    background: var(--device-2);
  }
  .slot-3 {
    background: var(--device-3);
  }
  .slot-4 {
    background: var(--device-4);
  }
  .slot-5 {
    background: var(--device-other);
  }
  .controls {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 30px;
    padding: 0 12px 0 9px;
    border: 1px solid var(--border);
    border-radius: 999px;
    font-size: var(--text-md);
    color: var(--text-dim);
    cursor: pointer;
    white-space: nowrap;
  }
  .toggle:has(input:checked) {
    background: var(--bg-active);
    border-color: var(--border-strong);
    color: var(--text);
  }
  .toggle input {
    width: 15px;
    height: 15px;
  }
  .linklike {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    color: var(--accent-text);
    cursor: pointer;
  }

  /* Feed */
  .feed {
    display: flex;
    flex-direction: column;
  }
  .day {
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-faint);
    margin: 18px 0 8px;
  }
  .day:first-child {
    margin-top: 0;
  }
  .entries {
    padding: 4px 0;
  }
  .entry {
    --tone: var(--text-faint);
    --tone-bg: rgba(158, 158, 168, 0.12);
    display: flex;
    align-items: baseline;
    gap: 12px;
    padding: 8px 16px;
    font-size: 0.88rem;
    line-height: 1.45;
  }
  .entry + .entry {
    border-top: 1px solid var(--border);
  }
  .entry.lvl-success {
    --tone: var(--success);
    --tone-bg: rgba(var(--success-rgb), 0.13);
  }
  .entry.lvl-warn {
    --tone: var(--warn);
    --tone-bg: rgba(var(--warn-rgb), 0.13);
  }
  .entry.lvl-error {
    --tone: var(--danger);
    --tone-bg: rgba(var(--danger-rgb), 0.16);
  }
  .icon {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    align-self: center;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    font-size: 0.7rem;
    font-weight: 800;
    color: var(--tone);
    background: var(--tone-bg);
  }
  .msg {
    flex: 1;
    min-width: 0;
    color: var(--text);
    word-break: break-word;
    user-select: text;
  }
  .lvl-warn .msg,
  .lvl-error .msg {
    color: var(--tone);
  }
  .game {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-weight: 600;
    color: inherit;
    cursor: pointer;
    text-decoration: underline;
    text-decoration-color: var(--border-strong);
    text-underline-offset: 2px;
  }
  .game:hover {
    color: var(--accent-text);
    text-decoration-color: currentColor;
  }
  .clock {
    flex-shrink: 0;
    font-size: 0.75rem;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }

  /* Technical log: the entries exactly as written, oldest first. */
  .log {
    height: calc(100vh - var(--titlebar-h) - var(--statusbar-h) - 130px);
    overflow-y: auto;
    font-family: 'Cascadia Code', 'Consolas', monospace;
    font-size: 0.8rem;
    padding: 14px;
  }
  .line {
    display: flex;
    gap: 10px;
    padding: 2px 0;
    align-items: baseline;
  }
  .stamp {
    color: var(--text-faint);
    flex-shrink: 0;
  }
  .level {
    width: 56px;
    flex-shrink: 0;
    font-weight: 600;
    color: var(--text-dim);
  }
  .level.lvl-success {
    color: var(--success);
  }
  .level.lvl-warn {
    color: var(--warn);
  }
  .level.lvl-error {
    color: var(--danger);
  }
  .raw {
    color: var(--text);
    word-break: break-word;
    user-select: text;
  }
</style>
