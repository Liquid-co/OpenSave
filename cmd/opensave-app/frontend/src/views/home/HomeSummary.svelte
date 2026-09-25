<script>
  // One sentence about the whole library, with the facts behind it beside it,
  // where three large counters used to be. Two of those counters — devices
  // online and syncs running — read 0 nearly all the time, which said
  // nothing, and none of them said the thing a person opens the app to
  // check: are my saves safe, and is anything waiting on me.
  //
  // The state is carried by an icon, not a coloured stripe and a dot: the
  // stripe and dot said "status" without saying which, and the same green dot
  // then repeated on every card below.
  import { onDestroy } from 'svelte';
  import ShieldCheck from 'lucide-svelte/icons/shield-check';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import CircleDashed from 'lucide-svelte/icons/circle-dashed';
  import CirclePause from 'lucide-svelte/icons/circle-pause';
  import ChevronRight from 'lucide-svelte/icons/chevron-right';
  import { peers, settings, navigate, syncPause } from '../../lib/stores.js';
  import { pauseLength, resumeSync } from '../../lib/syncpause.js';
  import { providerById } from '../../lib/cloudproviders.js';
  import { librarySummary, latestSnapshotAt } from '../../lib/gamestatus.js';
  import { spaceUsed } from '../../lib/overview.js';
  import { fmtSize } from '../../lib/format.js';
  import { timeAgo } from '../../lib/timeago.js';
  import { libraryView } from '../../lib/libraryview.js';
  import RecentActivity from './RecentActivity.svelte';

  /** [{game, status}] for every tracked game, from the library. */
  export let rows = [];

  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));

  const ICONS = { ok: ShieldCheck, warn: TriangleAlert, busy: RefreshCw, muted: CircleDashed };

  $: summary = librarySummary(rows);
  // The pause's countdown starts from the moment it changes.
  $: if ($syncPause) now = Date.now();
  // A pause outranks the quiet states, not a decision waiting or a sync in
  // flight: those still need seeing while it lasts.
  $: paused = $syncPause.paused;
  $: tone = paused && (summary.tone === 'ok' || summary.tone === 'muted') ? 'paused' : summary.tone;
  $: icon = tone === 'paused' ? CirclePause : ICONS[summary.tone];

  $: paired = Object.values($peers);
  $: online = paired.filter((p) => p.status === 'online');
  $: devices =
    paired.length === 0
      ? 'None paired'
      : paired.length === 1
        ? `${paired[0].name} · ${online.length ? 'online' : 'offline'}`
        : `${online.length} of ${paired.length} online`;

  $: cloudOn = !!$settings?.cloudSync?.enabled;
  $: cloud = cloudOn ? (providerById($settings.cloudSync.provider)?.label ?? 'On') : 'Off';

  $: space = spaceUsed(Object.fromEntries(rows.map((r) => [r.game.id, r.game])));

  $: newest = rows.reduce((best, r) => {
    const at = latestSnapshotAt(r.game);
    return at && (!best || at > best.at) ? { at, name: r.game.name } : best;
  }, null);
</script>

<section class="summary tone-{tone}">
  <div class="state">
    <div class="state-icon" class:spin={tone === 'busy'}><svelte:component this={icon} size={20} strokeWidth={2} /></div>
    <div class="state-text">
      <p class="headline" role="status">{summary.headline}</p>
      {#if paused}
        <p class="detail">
          Syncing is paused {pauseLength($syncPause, now)}. Snapshots are still taken, and everything catches up when it
          resumes.
        </p>
      {/if}
    </div>
    {#if paused}
      <button class="btn small" on:click={resumeSync}>Resume now</button>
    {/if}
  </div>

  <div class="facts">
    <button class="fact link" on:click={() => navigate('devices')} title="Open Devices">
      <span class="label">Devices</span>
      <span class="value" class:quiet={paired.length === 0}><span class="v">{devices}</span><ChevronRight size={13} class="go" /></span>
    </button>
    <button class="fact link" on:click={() => navigate('cloud')} title="Open Cloud Backup">
      <span class="label">Cloud backup</span>
      <span class="value" class:quiet={!cloudOn}><span class="v">{cloud}</span><ChevronRight size={13} class="go" /></span>
    </button>
    <div class="fact" title={newest ? `${newest.name} · ${new Date(newest.at).toLocaleString()}` : ''}>
      <span class="label">Last snapshot</span>
      <span class="value" class:quiet={!newest}><span class="v">{newest ? timeAgo(newest.at, now) : 'None yet'}</span></span>
    </div>
    <button class="fact link" on:click={() => navigate('settings', { tab: 'storage' })} title="Open Settings → Storage">
      <span class="label">Space used</span>
      <span class="value"><span class="v">{fmtSize(space)}</span><ChevronRight size={13} class="go" /></span>
    </button>
  </div>

  {#if $libraryView.overview}
    <RecentActivity />
  {/if}
</section>

<style>
  .summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 14px 28px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 16px 18px;
  }

  .state {
    display: flex;
    align-items: center;
    gap: 14px;
    min-width: 0;
    flex: 1 1 320px;
  }
  .state-icon {
    --tint: var(--success-rgb);
    width: 40px;
    height: 40px;
    border-radius: 11px;
    display: grid;
    place-items: center;
    flex-shrink: 0;
    color: rgb(var(--tint));
    background: rgba(var(--tint), 0.12);
  }
  .tone-warn .state-icon,
  .tone-paused .state-icon {
    --tint: var(--warn-rgb);
  }
  .tone-busy .state-icon {
    --tint: var(--accent-rgb);
  }
  .tone-muted .state-icon {
    color: var(--text-dim);
    background: var(--bg-active);
  }
  .state-icon.spin :global(svg) {
    animation: spin 1.6s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .state-icon.spin :global(svg) {
      animation: none;
    }
  }
  .state-text {
    min-width: 0;
  }
  .headline {
    font-size: 1.05rem;
    font-weight: 600;
    letter-spacing: -0.005em;
    line-height: 1.3;
  }
  .detail {
    margin-top: 3px;
    font-size: 0.84rem;
    color: var(--text-dim);
  }
  .state .btn {
    flex-shrink: 0;
  }

  .facts {
    display: flex;
    align-items: stretch;
    flex-wrap: wrap;
  }
  .fact {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
    padding: 6px 16px;
    border-left: 1px solid var(--border);
    text-align: left;
  }
  /* The row's ends sit on the card's edges, so the facts line up with the
     icon when they wrap below it in a narrow window. */
  .fact:first-child {
    border-left: none;
    padding-left: 0;
  }
  .fact:last-child {
    padding-right: 0;
  }
  .fact.link {
    background: none;
    border-top: none;
    border-right: none;
    border-bottom: none;
    border-radius: 0;
    font: inherit;
    color: inherit;
    cursor: pointer;
  }
  .label {
    font-size: 0.74rem;
    color: var(--text-faint);
  }
  .value {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    font-size: 0.9rem;
    font-weight: 500;
    color: var(--text);
    max-width: 220px;
  }
  .v {
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .value.quiet {
    color: var(--text-dim);
    font-weight: 400;
  }
  .value :global(.go) {
    flex-shrink: 0;
    color: var(--text-faint);
    opacity: 0;
    transform: translateX(-3px);
    transition:
      opacity 0.12s,
      transform 0.12s;
  }
  .fact.link:hover .value,
  .fact.link:focus-visible .value {
    color: var(--text);
  }
  .fact.link:hover :global(.go),
  .fact.link:focus-visible :global(.go) {
    opacity: 1;
    transform: none;
  }
</style>
