<script>
  // One sentence about the whole library and the facts behind it, where three
  // large counters used to be. Two of those counters — devices online and
  // syncs running — read 0 nearly all the time, which said nothing, and none
  // of them said the thing a person opens the app to check: are my saves
  // safe, and is anything waiting on me.
  import { onDestroy } from 'svelte';
  import { peers, settings, navigate, syncPause } from '../../lib/stores.js';
  import { pauseLength, resumeSync } from '../../lib/syncpause.js';
  import { providerById } from '../../lib/cloudproviders.js';
  import { librarySummary, latestSnapshotAt } from '../../lib/gamestatus.js';
  import { timeAgo } from '../../lib/timeago.js';

  /** [{game, status}] for every tracked game, from the library. */
  export let rows = [];

  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));

  $: summary = librarySummary(rows);
  // The pause's countdown starts from the moment it changes.
  $: if ($syncPause) now = Date.now();

  $: paired = Object.values($peers);
  $: online = paired.filter((p) => p.status === 'online').length;
  $: devices =
    paired.length === 0
      ? 'No other devices paired'
      : `${paired.length} ${paired.length === 1 ? 'device' : 'devices'} paired · ${online} online`;

  $: cloud = $settings?.cloudSync?.enabled
    ? `Cloud backup: ${providerById($settings.cloudSync.provider)?.label ?? 'on'}`
    : 'Cloud backup off';

  $: latest = rows.reduce((best, r) => {
    const t = latestSnapshotAt(r.game);
    return t && (!best || t > best) ? t : best;
  }, null);
</script>

<div class="summary tone-{summary.tone}">
  <div class="headline">
    <span class="dot"></span>
    {summary.headline}
  </div>
  {#if $syncPause.paused}
    <div class="paused-line">
      Syncing is paused {pauseLength($syncPause, now)} — snapshots are still taken, and everything catches up
      when it resumes.
      <button class="btn small" on:click={resumeSync}>Resume now</button>
    </div>
  {/if}
  <div class="facts">
    <span>{rows.length} {rows.length === 1 ? 'game' : 'games'}</span>
    <span class="sep">·</span>
    <button class="fact-link" on:click={() => navigate('devices')}>{devices}</button>
    <span class="sep">·</span>
    <button class="fact-link" on:click={() => navigate('cloud')}>{cloud}</button>
    {#if latest}
      <span class="sep">·</span>
      <span title={new Date(latest).toLocaleString()}>Latest snapshot {timeAgo(latest, now)}</span>
    {/if}
  </div>
</div>

<style>
  .paused-line {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
    margin: 2px 0 8px 17px;
    font-size: 0.86rem;
    color: var(--warn);
  }
  .summary {
    --tone: var(--success);
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-left: 3px solid var(--tone);
    border-radius: var(--radius-lg);
    padding: 16px 20px;
    margin-bottom: 26px;
  }
  .tone-warn {
    --tone: var(--warn);
  }
  .tone-busy {
    --tone: var(--accent);
  }
  .tone-muted {
    --tone: var(--text-faint);
  }
  .headline {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 1.05rem;
    font-weight: 600;
  }
  .dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--tone);
    flex-shrink: 0;
  }
  .tone-busy .dot {
    animation: pulse 1.4s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }
  .facts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 8px;
    margin: 6px 0 0 19px;
    font-size: 0.84rem;
    color: var(--text-dim);
  }
  .sep {
    color: var(--text-faint);
  }
  .fact-link {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    color: inherit;
    cursor: pointer;
  }
  .fact-link:hover {
    color: var(--text);
    text-decoration: underline;
  }
</style>
