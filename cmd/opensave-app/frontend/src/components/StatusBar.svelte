<script>
  // The bar along the bottom. On the left, what is happening now: the
  // connection lost, syncing paused, a game being played (its save waits for
  // the session to end, syncengine/playing.go), syncs running. On the right,
  // what is around you: each paired device, internet sync, the transfers,
  // and the version. Where syncing stands overall is at the top right
  // (SyncPanel), not repeated here.
  import { onMount, onDestroy } from 'svelte';
  import { wsConnected, syncActivity, wanRoom, peers, showAbout, syncPause, gameList, navigate } from '../lib/stores.js';
  import { resumeSync, pauseShort, pauseLength } from '../lib/syncpause.js';
  import { playLength } from '../lib/format.js';
  import { isHandheld } from '../lib/devices.js';
  import { gameCover } from '../lib/api.js';
  import Play from 'lucide-svelte/icons/play';
  import ArrowDownUp from 'lucide-svelte/icons/arrow-down-up';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Monitor from 'lucide-svelte/icons/monitor';
  import Globe from 'lucide-svelte/icons/globe';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import CoverImage from './CoverImage.svelte';
  import TransfersPanel from './TransfersPanel.svelte';
  import { native } from '../lib/api.js';
  import AboutModal from './AboutModal.svelte';
  import { t } from '../lib/i18n.js';
  import { rise, pop } from '../lib/motion.js';

  let transfersOpen = false;

  // Replaced by AppInfo() as soon as it answers; see FALLBACK_INFO.
  let version = 'dev';
  onMount(async () => {
    try {
      const info = await native.appInfo();
      if (info?.version) version = info.version;
    } catch {}
  });

  // The pause counts down, and a session's length grows, while on screen.
  let now = Date.now();
  const clock = setInterval(() => (now = Date.now()), 15_000);
  onDestroy(() => clearInterval(clock));
  $: if ($syncPause) now = Date.now();

  $: running = Object.entries($syncActivity).filter(([, s]) => s.state === 'running');
  // How far along what is moving is, taken together, for the line under
  // Transfers. Nothing known reads as just started.
  $: moving = running.length ? running.reduce((n, [, s]) => n + (s.percentage ?? 0), 0) / running.length / 100 : 0;
  // "Reconnecting" only once there has been a connection to lose: the socket
  // starts closed, and every launch would otherwise open by saying so.
  let connectedOnce = false;
  $: if ($wsConnected) connectedOnce = true;
  $: lost = connectedOnce && !$wsConnected;

  // Being played here, longest-running first.
  $: playing = $gameList.filter((g) => g.playingSince).sort((a, b) => Date.parse(a.playingSince) - Date.parse(b.playingSince));
  $: lead = playing[0];
  $: leadFor = lead ? Math.max(0, now - Date.parse(lead.playingSince)) : 0;

  // Online ones first; a few by name, the rest counted.
  const SHOWN = 3;
  $: paired = Object.values($peers).sort((a, b) => Number(b.status === 'online') - Number(a.status === 'online') || a.name.localeCompare(b.name));
  $: chips = paired.slice(0, SHOWN);
  $: more = paired.length - chips.length;

  $: internet = $wanRoom?.enabled ? ($wanRoom.connected ? 'on' : 'connecting') : null;
</script>

{#if $showAbout}
  <AboutModal onClose={() => showAbout.set(false)} />
{/if}

<svelte:window on:keydown={(e) => e.key === 'Escape' && (transfersOpen = false)} />

{#if transfersOpen}
  <TransfersPanel on:close={() => (transfersOpen = false)} />
{/if}

<footer aria-label={$t('shell.status.label')}>
  <div class="left">
    {#if lost}
      <span class="item warn" title={$t('statusBar.reconnectingTitle')}>{$t('statusBar.reconnecting')}</span>
    {:else if $syncPause.paused}
      <span class="item warn" title={$t('shell.status.pausedTitle', { length: pauseLength($syncPause, now) })}>
        <b>{$t('shell.status.paused')}</b>{pauseShort($syncPause, now)}
      </span>
      <button class="bar-btn" on:click={resumeSync} title={$t('statusBar.resumeTitle')}><Play size={11} />{$t('statusBar.resume')}</button>
    {:else}
      {#if lead}
        <button class="item link" on:click={() => navigate('game', { gameId: lead.id })} title={$t('shell.status.playingTitle')} in:rise={{ y: 10 }}>
          <span class="thumb"><CoverImage src={gameCover(lead, true)} alt="" revealed={false} /></span>
          <Gamepad2 size={14} class="pad" />
          <b>{$t('shell.status.playing', { name: lead.name })}</b>
          {#if leadFor >= 60_000}<span>{playLength(leadFor)}</span>{/if}
          {#if playing.length > 1}<span class="faint">{$t('shell.status.andMore', { n: playing.length - 1 })}</span>{/if}
          <span class="faint">{$t('shell.status.syncsWhenYouStop')}</span>
        </button>
      {/if}
      {#if running.length}
        <span class="item" in:rise={{ y: 10 }}>
          <RefreshCw size={12} class="turning" />
          {$t('statusBar.syncing', { n: running.length })}
          {#if running[0][1].percentage != null}<span class="pct">{running[0][1].percentage}%</span>{/if}
        </span>
      {/if}
    {/if}
  </div>

  <div class="right">
    {#if chips.length}
      <div class="chips" role="group" aria-label={$t('home.devices')}>
        {#each chips as p (p.id)}
          <button
            class="chip"
            in:pop={{ start: 0.85 }}
            class:offline={p.status !== 'online'}
            on:click={() => navigate('devices')}
            title={p.status === 'online' ? $t('shell.status.deviceOnline', { name: p.name }) : $t('shell.status.deviceOffline', { name: p.name })}
          >
            <svelte:component this={isHandheld(p.deviceType) ? Gamepad2 : Monitor} size={12} />
            {p.status === 'online' ? p.name : $t('shell.status.offlineChip', { name: p.name })}
          </button>
        {/each}
        {#if more > 0}<button class="chip more" on:click={() => navigate('devices')}>+{more}</button>{/if}
      </div>
      <span class="sep" aria-hidden="true"></span>
    {/if}
    {#if internet}
      <button class="bar-item" class:warn={internet === 'connecting'} on:click={() => navigate('devices')} title={$t('shell.status.internetTitle')}>
        <Globe size={13} />{internet === 'on' ? $t('shell.status.internetOn') : $t('shell.status.internetConnecting')}
      </button>
    {/if}
    <button
      class="bar-item transfers"
      class:on={transfersOpen}
      class:moving={running.length > 0}
      on:click={() => (transfersOpen = !transfersOpen)}
      title={$t('statusBar.transfersHint')}
      aria-expanded={transfersOpen}
    >
      <span class="swap"><ArrowDownUp size={13} /></span>{$t('transfers.title')}
      {#if running.length}
        {#key running.length}<span class="count" in:pop={{ start: 0.6 }}>{running.length}</span>{/key}
        <span class="moved" aria-hidden="true"><span style="transform: scaleX({Math.max(0.04, moving)})"></span></span>
      {/if}
    </button>
    <button class="bar-item ver" on:click={() => showAbout.set(true)} title={$t('statusBar.aboutTitle')}>{version}</button>
  </div>
</footer>

<style>
  footer {
    height: var(--statusbar-h);
    display: flex;
    align-items: stretch;
    justify-content: space-between;
    gap: 12px;
    padding: 0 6px;
    background: var(--bg-sidebar);
    border-top: 1px solid var(--border);
    font-size: var(--text-sm);
    color: var(--text-dim);
    flex-shrink: 0;
    min-width: 0;
  }
  .left,
  .right {
    display: flex;
    align-items: center;
    gap: 2px;
    min-width: 0;
  }
  .left {
    flex: 1 1 auto;
    overflow: hidden;
  }
  .right {
    flex-shrink: 0;
  }
  .item {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    height: 100%;
    padding: 0 8px;
    white-space: nowrap;
    min-width: 0;
  }
  .item b {
    font-weight: 600;
    color: var(--text);
  }
  .item.warn,
  .item.warn b {
    color: var(--warn);
  }
  .item.link {
    border: none;
    background: transparent;
    font: inherit;
    color: inherit;
    cursor: pointer;
    overflow: hidden;
  }
  .item.link:hover {
    background: var(--bg-hover);
  }
  .item :global(.pad) {
    color: var(--success);
    flex-shrink: 0;
  }
  .faint {
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .thumb {
    position: relative;
    width: 18px;
    height: 24px;
    border-radius: 3px;
    overflow: hidden;
    flex-shrink: 0;
    background: var(--bg-active);
  }
  .thumb :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .pct {
    color: var(--accent-text);
    font-weight: 600;
  }
  .item :global(.turning) {
    color: var(--accent-text);
    animation: turn 1.4s linear infinite;
  }
  @keyframes turn {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .item :global(.turning) {
      animation: none;
    }
  }
  .chips {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 8px;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 22px;
    padding: 0 8px;
    border: 1px solid transparent;
    border-radius: 6px;
    background: var(--bg-active);
    color: var(--text);
    font: inherit;
    font-size: var(--text-sm);
    white-space: nowrap;
    cursor: pointer;
  }
  .chip :global(svg) {
    color: var(--success);
  }
  .chip.offline {
    background: transparent;
    border: 1px dashed var(--border-strong);
    color: var(--text-faint);
  }
  .chip.offline :global(svg) {
    color: var(--text-faint);
  }
  .chip:hover {
    border-color: var(--border-strong);
  }
  .chip,
  .chip :global(svg) {
    transition:
      background 0.3s,
      border-color 0.3s,
      color 0.3s;
  }
  .bar-item,
  .item.link {
    transition:
      background 0.12s,
      color 0.12s;
  }
  .sep {
    width: 1px;
    height: 16px;
    align-self: center;
    background: var(--border-strong);
    margin: 0 4px;
  }
  .bar-item,
  .bar-btn {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 100%;
    padding: 0 10px;
    border: none;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-sm);
    white-space: nowrap;
    cursor: pointer;
  }
  .bar-btn {
    height: 22px;
    align-self: center;
    border: 1px solid var(--border-strong);
    border-radius: 6px;
  }
  .bar-item:hover,
  .bar-item.on,
  .bar-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .bar-item.warn {
    color: var(--warn);
  }
  /* Transfers, while something moves: the arrows trade places, it takes the
     colour of things on their way, and a line along its foot fills. */
  .transfers {
    position: relative;
  }
  .swap {
    display: grid;
  }
  .transfers.moving {
    color: var(--info);
  }
  .transfers.moving .swap :global(svg) {
    animation: trade 1.6s ease-in-out infinite;
  }
  @keyframes trade {
    50% {
      transform: rotate(180deg);
    }
  }
  .moved {
    position: absolute;
    left: 8px;
    right: 8px;
    bottom: 3px;
    height: 2px;
    border-radius: 2px;
    background: rgba(var(--info-rgb), 0.2);
    overflow: hidden;
  }
  .moved span {
    display: block;
    height: 100%;
    background: var(--info);
    transform-origin: left;
    transition: transform 0.4s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .count {
    padding: 0 6px;
    border-radius: 999px;
    background: var(--accent-fill);
    color: var(--on-accent);
    font-size: var(--text-xs);
    font-weight: 700;
  }
  .ver {
    color: var(--text-faint);
  }
  /* Narrow windows: the reassurance goes before the facts do. */
  @media (max-width: 1180px) {
    .item.link .faint:last-child {
      display: none;
    }
  }
  @media (max-width: 980px) {
    .chips {
      display: none;
    }
    .sep {
      display: none;
    }
  }
</style>
