<script>
  import { onMount, onDestroy } from 'svelte';
  import { wsConnected, syncActivity, wanRoom, peers, showAbout, syncPause } from '../lib/stores.js';
  import { PAUSE_CHOICES, pauseSync, resumeSync, pauseShort, pauseLength } from '../lib/syncpause.js';
  import { openMenu } from '../lib/contextmenu.js';
  import Pause from 'lucide-svelte/icons/pause';
  import Play from 'lucide-svelte/icons/play';
  import ArrowDownUp from 'lucide-svelte/icons/arrow-down-up';
  import TransfersPanel from './TransfersPanel.svelte';

  let transfersOpen = false;
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

  // The pause counts down while it is on screen.
  let now = Date.now();
  const clock = setInterval(() => (now = Date.now()), 15_000);
  onDestroy(() => clearInterval(clock));
  // And starts from the moment it changes, not from the last tick.
  $: if ($syncPause) now = Date.now();

  const pauseMenu = (e) =>
    openMenu(
      e,
      PAUSE_CHOICES.map((c) => ({ label: `Pause syncing ${c.label}`, icon: Pause, run: () => pauseSync(c.minutes) }))
    );

  $: running = Object.entries($syncActivity).filter(([, s]) => s.state === 'running');
  // "Reconnecting" only once there has been a connection to lose: the socket
  // starts closed, and every launch would otherwise open by saying so.
  let connectedOnce = false;
  $: if ($wsConnected) connectedOnce = true;
  $: lost = connectedOnce && !$wsConnected;
  $: onlinePeers = Object.values($peers).filter((p) => p.status === 'online').length;
  $: statusText = running.length
    ? `Syncing ${running.length} game${running.length > 1 ? 's' : ''}…`
    : 'No syncs in progress';
</script>

{#if $showAbout}
  <AboutModal onClose={() => showAbout.set(false)} />
{/if}

<svelte:window on:keydown={(e) => e.key === 'Escape' && (transfersOpen = false)} />

{#if transfersOpen}
  <TransfersPanel on:close={() => (transfersOpen = false)} />
{/if}

<footer>
  <div class="left">
    {#if lost}
      <span class="offline" title="The app has lost touch with its background service and is reconnecting">Reconnecting…</span>
    {:else if $syncPause.paused}
      <span class="paused" title="Syncing is paused {pauseLength($syncPause, now)}. Snapshots are still taken.">
        Syncing paused · {pauseShort($syncPause, now)}
      </span>
      <button class="bar-btn" on:click={resumeSync} title="Resume syncing and catch up"><Play size={11} />Resume</button>
    {:else}
      <span>{statusText}</span>
      {#if running.length && running[0][1].percentage != null}
        <span class="pct">{running[0][1].percentage}%</span>
      {/if}
      <button class="bar-btn icon" on:click={pauseMenu} title="Pause syncing" aria-label="Pause syncing"><Pause size={11} /></button>
    {/if}
  </div>
  <div class="right">
    {#if $wanRoom?.connected}
      <span class="wan">relay: {$wanRoom.roomCode}</span>
    {/if}
    <span>{onlinePeers} peer{onlinePeers === 1 ? '' : 's'} online</span>
    <button
      class="bar-btn"
      class:on={transfersOpen}
      on:click={() => (transfersOpen = !transfersOpen)}
      title="What is moving between your devices"
      aria-expanded={transfersOpen}
    >
      <ArrowDownUp size={11} />Transfers{#if running.length}<span class="count">{running.length}</span>{/if}
    </button>
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
  .offline {
    color: var(--warn);
  }
  .paused {
    color: var(--warn);
    font-weight: 600;
  }
  .bar-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    height: 18px;
    padding: 0 7px;
    border: 1px solid var(--border-strong);
    border-radius: 5px;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: 0.7rem;
    cursor: pointer;
  }
  .bar-btn.icon {
    padding: 0 4px;
    border-color: transparent;
    color: var(--text-faint);
  }
  .bar-btn:hover,
  .bar-btn.on {
    color: var(--text);
    background: var(--bg-hover);
  }
  .count {
    margin-left: 2px;
    padding: 0 5px;
    border-radius: 999px;
    background: var(--accent);
    color: #fff;
    font-size: 0.65rem;
    font-weight: 700;
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
