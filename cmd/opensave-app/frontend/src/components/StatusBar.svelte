<script>
  import { onMount } from 'svelte';
  import { wsConnected, syncActivity, wanRoom, peers, showAbout } from '../lib/stores.js';
  import { native } from '../lib/api.js';
  import AboutModal from './AboutModal.svelte';
  import { paletteOpen } from '../lib/shortcuts.js';

  const modKey = /Mac/i.test(globalThis.navigator?.platform ?? '') ? '⌘' : 'Ctrl';

  // Replaced by AppInfo() as soon as it answers; see FALLBACK_INFO.
  let version = 'dev';
  onMount(async () => {
    try {
      const info = await native.appInfo();
      if (info?.version) version = info.version;
    } catch {}
  });

  $: running = Object.entries($syncActivity).filter(([, s]) => s.state === 'running');
  $: onlinePeers = Object.values($peers).filter((p) => p.status === 'online').length;
  $: statusText = running.length
    ? `Syncing ${running.length} game${running.length > 1 ? 's' : ''}…`
    : 'No syncs in progress';
</script>

{#if $showAbout}
  <AboutModal onClose={() => showAbout.set(false)} />
{/if}

<footer>
  <div class="left">
    <span class="dot" class:green={$wsConnected} class:gray={!$wsConnected}></span>
    <span>{statusText}</span>
    {#if running.length && running[0][1].percentage != null}
      <span class="pct">{running[0][1].percentage}%</span>
    {/if}
  </div>
  <div class="right">
    {#if $wanRoom?.connected}
      <span class="wan">relay: {$wanRoom.roomCode}</span>
    {/if}
    <span>{onlinePeers} peer{onlinePeers === 1 ? '' : 's'} online</span>
    <button class="kbd-hint" on:click={() => paletteOpen.set(true)} title="Jump to a game, a page or an action">
      <kbd>{modKey} K</kbd>
    </button>
    <button class="ver" on:click={() => showAbout.set(true)} title="About OpenSave">OpenSave v{version}</button>
  </div>
</footer>

<style>
  footer {
    height: var(--statusbar-h);
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 14px;
    background: var(--bg-sidebar);
    border-top: 1px solid var(--border);
    font-size: 0.76rem;
    color: var(--text-faint);
    flex-shrink: 0;
  }
  .left,
  .right {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .pct {
    color: var(--accent);
    font-weight: 600;
  }
  .wan {
    color: var(--success);
  }
  .kbd-hint {
    border: none;
    background: transparent;
    padding: 0;
    cursor: pointer;
  }
  .kbd-hint kbd {
    font: inherit;
    font-size: 0.7rem;
    padding: 1px 6px;
    border: 1px solid var(--border-strong);
    border-radius: 5px;
    color: var(--text-faint);
  }
  .kbd-hint:hover kbd {
    color: var(--text);
    border-color: var(--text-faint);
  }
  .ver {
    opacity: 0.7;
    border: none;
    background: transparent;
    color: var(--text-faint);
    font-size: 0.76rem;
    cursor: pointer;
    padding: 2px 4px;
    border-radius: 5px;
  }
  .ver:hover {
    opacity: 1;
    background: var(--bg-hover);
    color: var(--text-dim);
  }
</style>
