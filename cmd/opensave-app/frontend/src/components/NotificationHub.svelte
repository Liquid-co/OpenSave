<script>
  // The bell in the title bar, and what is behind it: everything waiting on
  // you, and what happened lately, with a count of both on the bell. See
  // lib/notifications.js for what goes in and when it counts as read.
  import { onDestroy } from 'svelte';
  import Bell from 'lucide-svelte/icons/bell';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import Info from 'lucide-svelte/icons/info';
  import CoverImage from './CoverImage.svelte';
  import { api, gameCover } from '../lib/api.js';
  import { stateLoaded, games, conflicts, locationConflicts, pairingRequests, cloudOffers, newGames, navigate, activityTick, availableUpdate } from '../lib/stores.js';
  import { waitingOnYou, happened, badgeCount, loadSeenAt, saveSeenAt } from '../lib/notifications.js';
  import { timeAgo } from '../lib/timeago.js';

  let open = false;
  let items = [];
  let seenAt = loadSeenAt();
  let bell;
  let panel;

  async function load() {
    try {
      items = (await api.get('/api/activity?limit=120')).items ?? [];
    } catch {
      // Nothing to add: the questions still show.
    }
  }
  // Asked once the app has its state: the bell is drawn before the daemon
  // is reached, and a request then would fail and leave the list empty.
  const stopLoaded = stateLoaded.subscribe((ready) => ready && load());
  let pending = null;
  const stop = activityTick.subscribe((n) => {
    if (!n) return;
    clearTimeout(pending);
    pending = setTimeout(load, 700);
  });
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 60_000);
  onDestroy(() => {
    stopLoaded();
    stop();
    clearTimeout(pending);
    clearInterval(tick);
  });

  $: waiting = waitingOnYou({
    games: $games,
    conflicts: $conflicts,
    locationConflicts: $locationConflicts,
    pairingRequests: $pairingRequests,
    cloudOffers: $cloudOffers,
    newGames: $newGames,
    update: $availableUpdate
  });
  $: events = happened(items, $games, seenAt, now);
  $: count = badgeCount(waiting, events);

  // Opened and then closed, everything shown has been seen. Marked on
  // closing, so the dots are still there to read while the panel is open.
  function toggle() {
    if (open) close();
    else open = true;
  }
  function close() {
    open = false;
    seenAt = Date.now();
    saveSeenAt(seenAt);
  }
  function markAllRead() {
    seenAt = Date.now();
    saveSeenAt(seenAt);
  }
  function go(item) {
    close();
    if (item.go) navigate(item.go.view, item.go.params ?? {});
  }
  function outside(e) {
    if (open && !panel?.contains(e.target) && !bell?.contains(e.target)) close();
  }
  const initials = (name = '') => name.split(/\s+/).map((w) => w[0]).join('').slice(0, 2).toUpperCase();
  const iso = (ms) => new Date(ms).toISOString();
</script>

<svelte:window on:mousedown={outside} on:keydown={(e) => open && e.key === 'Escape' && close()} />

<div class="hub" style="--wails-draggable: no-drag">
  <button
    bind:this={bell}
    class="bell"
    class:has={count > 0}
    on:click={toggle}
    title={count ? `${count} notification${count === 1 ? '' : 's'}` : 'Notifications'}
    aria-label={count ? `Notifications, ${count} new` : 'Notifications'}
    aria-expanded={open}
  >
    <Bell size={14} />
    {#if count}<span class="badge">{count > 9 ? '9+' : count}</span>{/if}
  </button>

  {#if open}
    <div class="panel" bind:this={panel} role="dialog" aria-label="Notifications">
      <div class="head">
        <span class="title">Notifications</span>
        {#if events.some((e) => e.unread)}
          <button class="linklike" on:click={markAllRead}>Mark all as read</button>
        {/if}
      </div>
      <div class="scroll">
        {#if waiting.length}
          <div class="section">Needs you</div>
          {#each waiting as w (w.id)}
            <button class="item tone-{w.tone}" on:click={() => go(w)}>
              {#if w.gameId && $games[w.gameId]}
                <span class="thumb"><span class="initials">{initials($games[w.gameId].name)}</span><CoverImage src={gameCover($games[w.gameId])} alt="" /></span>
              {:else}
                <span class="thumb icon"><svelte:component this={w.tone === 'warn' ? TriangleAlert : Info} size={15} /></span>
              {/if}
              <span class="text"><span class="t">{w.title}</span><span class="d">{w.detail}</span></span>
            </button>
          {/each}
        {/if}
        {#if events.length}
          <div class="section">Earlier</div>
          {#each events as e (e.id)}
            <button class="item" class:unread={e.unread} on:click={() => go(e)}>
              <span class="thumb"><span class="initials">{initials($games[e.gameId]?.name)}</span><CoverImage src={gameCover($games[e.gameId])} alt="" /></span>
              <span class="text">
                <span class="t">{e.title}</span>
                <span class="d">{e.detail ? `${e.detail} · ` : ''}{timeAgo(iso(e.atMs), now)}</span>
              </span>
              {#if e.unread}<span class="dot" aria-label="Unread"></span>{/if}
            </button>
          {/each}
        {/if}
        {#if !waiting.length && !events.length}
          <div class="empty">Nothing new. Syncs, saves from your other devices and anything that needs you show up here.</div>
        {/if}
      </div>
      <div class="foot">
        <button class="linklike" on:click={() => go({ go: { view: 'activity' } })}>See all activity</button>
      </div>
    </div>
  {/if}
</div>

<style>
  .hub {
    position: relative;
    height: 100%;
    display: flex;
    align-items: center;
  }
  .bell {
    position: relative;
    width: 34px;
    height: 26px;
    margin-right: 6px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text-dim);
    display: grid;
    place-items: center;
    cursor: pointer;
  }
  .bell:hover,
  .bell[aria-expanded='true'] {
    background: var(--bg-hover);
    color: var(--text);
  }
  .badge {
    position: absolute;
    top: 1px;
    right: 3px;
    min-width: 15px;
    height: 15px;
    padding: 0 4px;
    border-radius: 8px;
    background: var(--accent);
    color: #fff;
    font-size: 0.62rem;
    font-weight: 700;
    line-height: 15px;
    text-align: center;
  }
  .panel {
    position: absolute;
    top: calc(100% + 4px);
    right: 0;
    z-index: 60;
    width: 380px;
    max-height: min(560px, 75vh);
    display: flex;
    flex-direction: column;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: 12px;
    box-shadow: 0 16px 40px rgba(0, 0, 0, 0.45);
    overflow: hidden;
  }
  .head,
  .foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 14px;
  }
  .head {
    border-bottom: 1px solid var(--border);
  }
  .foot {
    border-top: 1px solid var(--border);
  }
  .title {
    font-weight: 600;
    font-size: 0.9rem;
  }
  .scroll {
    overflow-y: auto;
    padding: 4px 6px 8px;
  }
  .section {
    margin: 10px 8px 4px;
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--text-faint);
  }
  .item {
    position: relative;
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 8px;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .item:hover {
    background: var(--bg-hover);
  }
  .thumb {
    position: relative;
    flex: none;
    width: 32px;
    height: 32px;
    border-radius: 7px;
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
  .thumb.icon {
    color: var(--text-dim);
  }
  .tone-warn .thumb.icon {
    color: var(--warn);
  }
  .initials {
    font-size: 0.66rem;
    font-weight: 700;
    color: var(--text-faint);
  }
  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .t {
    font-size: 0.84rem;
    color: var(--text-dim);
  }
  .unread .t,
  .tone-warn .t {
    color: var(--text);
    font-weight: 600;
  }
  .d {
    font-size: 0.75rem;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dot {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
  }
  .empty {
    padding: 28px 16px;
    text-align: center;
    font-size: 0.84rem;
    color: var(--text-faint);
  }
  .linklike {
    border: none;
    background: none;
    padding: 0;
    color: var(--accent-hover);
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .linklike:hover {
    text-decoration: underline;
  }
</style>
