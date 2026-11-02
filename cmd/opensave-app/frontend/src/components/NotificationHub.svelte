<script>
  // The bell in the title bar, and what is behind it: everything waiting on
  // you, and what happened lately, with a count of both on the bell. See
  // lib/notifications.js for what goes in and when it counts as read.
  import { onDestroy } from 'svelte';
  import Bell from 'lucide-svelte/icons/bell';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import Info from 'lucide-svelte/icons/info';
  import X from 'lucide-svelte/icons/x';
  import CoverImage from './CoverImage.svelte';
  import { unfold } from '../lib/motion.js';
  import { api, gameCover } from '../lib/api.js';
  import { stateLoaded, games, conflicts, locationConflicts, pairingRequests, cloudOffers, newGames, navigate, activityTick, availableUpdate } from '../lib/stores.js';
  import {
    waitingOnYou,
    happened,
    badgeCount,
    loadSeenAt,
    saveSeenAt,
    exampleEvents,
    loadCleared,
    saveCleared,
    canDismiss,
    notCleared,
    dismissOne,
    clearAllHappened
  } from '../lib/notifications.js';
  import { timeAgo } from '../lib/timeago.js';
  import { t as translateUI } from '../lib/i18n.js';

  let open = false;
  let items = [];
  let seenAt = loadSeenAt();
  let cleared = loadCleared();
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

  $: allWaiting = ($translateUI, waitingOnYou({
    games: $games,
    conflicts: $conflicts,
    locationConflicts: $locationConflicts,
    pairingRequests: $pairingRequests,
    cloudOffers: $cloudOffers,
    newGames: $newGames,
    update: $availableUpdate
  }));
  $: allEvents = [...$exampleEvents, ...happened(items, $games, seenAt, now)];
  // What is shown, and counted on the bell: what has not been cleared.
  $: waiting = notCleared(allWaiting, cleared);
  $: events = notCleared(allEvents, cleared);
  $: count = badgeCount(waiting, events);
  $: listed = new Set([...allWaiting, ...allEvents].map((it) => it.id));

  function dismiss(item) {
    cleared = dismissOne(cleared, item.id, listed);
    saveCleared(cleared);
  }
  function clearAll() {
    cleared = clearAllHappened(cleared, Date.now(), listed);
    saveCleared(cleared);
    exampleEvents.set([]);
  }

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
    exampleEvents.set([]); // seen, and only ever an example
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
    title={count ? $translateUI('notifications.countTitle', { n: count }) : $translateUI('notifications.title')}
    aria-label={count ? $translateUI('notifications.countAria', { n: count }) : $translateUI('notifications.title')}
    aria-expanded={open}
  >
    <Bell size={14} />
    {#if count}{#key count}<span class="badge">{count > 9 ? '9+' : count}</span>{/key}{/if}
  </button>

  {#if open}
    <!-- Unfolds out of the bell and folds back into it (lib/motion.js). -->
    <div class="panel" bind:this={panel} role="dialog" aria-label={$translateUI('notifications.title')} transition:unfold>
      <div class="head">
        <span class="title">{$translateUI('notifications.title')}</span>
        {#if events.some((e) => e.unread)}
          <button class="linklike" on:click={markAllRead}>{$translateUI('notifications.markAllRead')}</button>
        {/if}
      </div>
      <div class="scroll">
        {#if waiting.length}
          <div class="section">{$translateUI('notifications.needsYou')}</div>
          {#each waiting as w, i (w.id)}
            <div class="row">
            <button class="item tone-{w.tone}" class:dismissable={canDismiss(w)} style="--i: {i}" on:click={() => go(w)}>
              {#if w.gameId && $games[w.gameId]}
                <span class="thumb"><span class="initials">{initials($games[w.gameId].name)}</span><CoverImage src={gameCover($games[w.gameId])} alt="" /></span>
              {:else}
                <span class="thumb icon"><svelte:component this={w.tone === 'warn' ? TriangleAlert : Info} size={15} /></span>
              {/if}
              <span class="text"><span class="t">{w.title}</span><span class="d">{w.detail}</span></span>
            </button>
            {#if canDismiss(w)}
              <button class="dismiss" title="{$translateUI('common.dismiss')}" aria-label="{$translateUI('notificationHub.dismiss')} {w.title}" on:click={() => dismiss(w)}><X size={13} /></button>
            {/if}
            </div>
          {/each}
        {/if}
        {#if events.length}
          <div class="section section-row">
            <span>{$translateUI('notifications.earlier')}</span>
            <button class="linklike" on:click={clearAll} title={$translateUI('notifications.clearAllTitle')}>{$translateUI('notifications.clearAll')}</button>
          </div>
          {#each events as e, i (e.id)}
            <div class="row">
            <button class="item dismissable" class:unread={e.unread} style="--i: {Math.min(waiting.length + i, 10)}" on:click={() => go(e)}>
              {#if e.example}
                <span class="thumb icon"><Info size={15} /></span>
              {:else}
                <span class="thumb"><span class="initials">{initials($games[e.gameId]?.name)}</span><CoverImage src={gameCover($games[e.gameId])} alt="" /></span>
              {/if}
              <span class="text">
                <span class="t">{e.title}</span>
                <span class="d">{e.detail ? `${e.detail} · ` : ''}{timeAgo(iso(e.atMs), now)}</span>
              </span>
              {#if e.unread}<span class="dot" aria-label={$translateUI('notifications.unread')}></span>{/if}
            </button>
            <button class="dismiss" title="{$translateUI('common.dismiss')}" aria-label="{$translateUI('notificationHub.dismiss')} {e.title}" on:click={() => dismiss(e)}><X size={13} /></button>
            </div>
          {/each}
        {/if}
        {#if !waiting.length && !events.length}
          <div class="empty">{$translateUI('notifications.empty')}</div>
        {/if}
      </div>
      <div class="foot">
        <button class="linklike" on:click={() => go({ go: { view: 'activity' } })}>{$translateUI('notifications.seeAll')}</button>
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
    transition:
      background 0.12s,
      color 0.12s,
      transform 0.16s cubic-bezier(0.2, 0.7, 0.2, 1.4);
  }
  .bell:active {
    transform: scale(0.9);
    transition-duration: 0.06s;
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
    color: var(--on-accent);
    font-size: 0.62rem;
    font-weight: 700;
    line-height: 15px;
    text-align: center;
    /* Keyed on the count, so it pops each time the number changes. */
    animation: badge-pop 0.3s cubic-bezier(0.2, 0.7, 0.2, 1.5) backwards;
  }
  @keyframes badge-pop {
    from {
      transform: scale(0.4);
    }
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
    /* One after another as the panel opens, and a new one as it arrives. */
    animation: item-in 0.22s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(40ms + var(--i, 0) * 24ms);
  }
  @keyframes item-in {
    from {
      opacity: 0;
      transform: translateY(5px);
    }
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
    color: var(--accent-text);
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .linklike:hover {
    text-decoration: underline;
  }
  .section-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .section-row .linklike {
    text-transform: none;
    letter-spacing: normal;
    font-weight: normal;
  }
  .row {
    position: relative;
  }
  .item.dismissable {
    padding-right: 34px;
  }
  /* Shown on pointing at the row or reaching it by keyboard, so the list
     stays as quiet as it was. */
  .dismiss {
    position: absolute;
    right: 6px;
    top: 50%;
    transform: translateY(-50%);
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    border: none;
    border-radius: 6px;
    background: none;
    color: var(--text-dim);
    cursor: pointer;
    opacity: 0;
  }
  .row:hover .dismiss,
  .row:focus-within .dismiss {
    opacity: 1;
  }
  .dismiss:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
</style>
