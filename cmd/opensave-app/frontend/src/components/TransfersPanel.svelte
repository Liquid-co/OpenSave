<script>
  // What is moving between this device and others, and what moved lately.
  // Opens from the status bar; refreshes whenever a sync reports progress.
  import { createEventDispatcher, onDestroy, onMount } from 'svelte';
  import ArrowDown from 'lucide-svelte/icons/arrow-down';
  import ArrowUp from 'lucide-svelte/icons/arrow-up';
  import CircleCheck from 'lucide-svelte/icons/circle-check';
  import CircleX from 'lucide-svelte/icons/circle-x';
  import X from 'lucide-svelte/icons/x';
  import { api } from '../lib/api.js';
  import { games, syncActivity, navigate } from '../lib/stores.js';
  import { timeAgo } from '../lib/timeago.js';
  import { amountLabel, peerLabel, speedLabel, withNames } from '../lib/transfers.js';

  const dispatch = createEventDispatcher();
  let data = null;
  let error = '';
  let now = Date.now();

  async function load() {
    try {
      data = await api.get('/api/transfers');
      error = '';
    } catch (e) {
      error = e.message;
    }
    now = Date.now();
  }
  onMount(load);
  // Any sync event means the list has changed.
  const unsub = syncActivity.subscribe(() => load());
  // And while something runs, its progress moves on its own.
  const poll = setInterval(() => data?.active?.length && load(), 2000);
  onDestroy(() => {
    unsub();
    clearInterval(poll);
  });

  $: shown = withNames(data, $games);

  function open(t) {
    if ($games[t.gameId]) {
      dispatch('close');
      navigate('game', { gameId: t.gameId });
    }
  }
</script>

<div class="panel" role="dialog" aria-label="Transfers">
  <div class="head">
    <h3>Transfers</h3>
    <button class="btn small ghost icon" aria-label="Close" title="Close" on:click={() => dispatch('close')}><X size={14} /></button>
  </div>

  {#if error}
    <p class="empty">Couldn't read transfers: {error}</p>
  {:else if !data}
    <div class="skeleton" style="height: 44px; margin: 10px 14px"></div>
  {:else if shown.active.length === 0 && shown.recent.length === 0}
    <p class="empty">Nothing has moved between your devices since OpenSave started.</p>
  {:else}
    {#if shown.active.length}
      <h4>Now</h4>
      {#each shown.active as t}
        <button class="row" on:click={() => open(t)}>
          <span class="dir {t.direction}"><svelte:component this={t.direction === 'upload' ? ArrowUp : ArrowDown} size={14} /></span>
          <span class="main">
            <span class="name">{t.name}</span>
            <span class="sub">{peerLabel(t)}{#if amountLabel(t)} · {amountLabel(t)}{/if}{#if speedLabel(t.speedBytesPerSec)} · {speedLabel(t.speedBytesPerSec)}{/if}</span>
            <span class="bar"><span style="width: {Math.max(3, t.percentage ?? 0)}%"></span></span>
          </span>
          <span class="pct">{t.percentage ?? 0}%</span>
        </button>
      {/each}
    {/if}
    {#if shown.recent.length}
      <h4>Recently</h4>
      {#each shown.recent.slice(0, 20) as t}
        <button class="row" on:click={() => open(t)} title={t.error || ''}>
          <span class="dir {t.direction}"><svelte:component this={t.direction === 'upload' ? ArrowUp : ArrowDown} size={14} /></span>
          <span class="main">
            <span class="name">{t.name}</span>
            <span class="sub">{peerLabel(t)}{#if amountLabel(t)} · {amountLabel(t)}{/if}{#if t.error} · {t.error}{/if}</span>
          </span>
          <span class="end">
            {#if t.state === 'error'}<span class="bad"><CircleX size={13} /></span>{:else}<span class="good"><CircleCheck size={13} /></span>{/if}
            <span class="when">{t.endedAt ? timeAgo(t.endedAt, now) : ''}</span>
          </span>
        </button>
      {/each}
    {/if}
  {/if}
</div>

<style>
  .panel {
    position: fixed;
    right: 12px;
    bottom: calc(var(--statusbar-h) + 8px);
    z-index: 120;
    width: min(380px, calc(100vw - 24px));
    max-height: min(60vh, 520px);
    overflow-y: auto;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
    padding-bottom: 8px;
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 10px 6px 14px;
  }
  h3 {
    font-size: 0.95rem;
  }
  h4 {
    margin: 8px 14px 4px;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
  }
  .empty {
    padding: 8px 14px 12px;
    font-size: 0.85rem;
    color: var(--text-dim);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 7px 14px;
    border: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .row:hover {
    background: var(--bg-hover);
  }
  .dir {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    flex-shrink: 0;
    border-radius: 8px;
    background: var(--btn-bg);
    color: var(--text-dim);
  }
  .dir.download {
    color: var(--accent);
  }
  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .name {
    font-size: 0.86rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .sub {
    font-size: 0.74rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .bar {
    height: 4px;
    margin-top: 3px;
    border-radius: 999px;
    background: var(--bg-active);
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    background: var(--accent);
    transition: width 0.3s ease;
  }
  .pct,
  .when {
    font-size: 0.74rem;
    color: var(--text-faint);
    white-space: nowrap;
  }
  .end {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .good {
    display: flex;
    color: var(--success);
  }
  .bad {
    display: flex;
    color: var(--danger);
  }
</style>
