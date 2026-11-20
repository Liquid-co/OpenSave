<script>
  // The button at the top right of the title bar that says where syncing
  // stands, and the panel it opens: every device and the cloud, each with
  // where it is, and the two things you do from here — sync everything now,
  // or pause. The same on every page; the Library's own Sync button is for
  // its games (lib/libraryhome.js syncState says which state is shown).
  import { onDestroy } from 'svelte';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import CircleCheck from 'lucide-svelte/icons/circle-check';
  import CirclePause from 'lucide-svelte/icons/circle-pause';
  import CloudOff from 'lucide-svelte/icons/cloud-off';
  import Unplug from 'lucide-svelte/icons/unplug';
  import Pause from 'lucide-svelte/icons/pause';
  import Play from 'lucide-svelte/icons/play';
  import Cloud from 'lucide-svelte/icons/cloud';
  import ChevronDown from 'lucide-svelte/icons/chevron-down';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Monitor from 'lucide-svelte/icons/monitor';
  import { isHandheld } from '../lib/devices.js';
  import { unfold, pop, rise } from '../lib/motion.js';
  import { peers, syncActivity, syncPause, settings, gameList, navigate, toast } from '../lib/stores.js';
  import { syncState } from '../lib/libraryhome.js';
  import { syncPanelOpen } from '../lib/shell.js';
  import { syncAllGames } from '../lib/gameactions.js';
  import { PAUSE_CHOICES, pauseSync, resumeSync, pauseLength } from '../lib/syncpause.js';
  import { providerById } from '../lib/cloudproviders.js';
  import { openMenu } from '../lib/contextmenu.js';
  import { timeAgoParts } from '../lib/timeago.js';
  import { t } from '../lib/i18n.js';

  // timeAgoParts reads ISO strings; the times here are milliseconds.
  const agoKey = (at, now) => {
    const p = timeAgoParts(typeof at === 'number' ? new Date(at).toISOString() : at, now);
    if (p.kind === 'invalid') return { text: p.text };
    const { kind, ...params } = p;
    return { key: 'timeAgo.' + kind, params };
  };
  const said = (s) => (s.key ? $t(s.key, s.params) : s.text);

  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));
  $: if ($syncPause || $syncPanelOpen) now = Date.now();

  $: state = syncState({ peers: $peers, activity: $syncActivity, paused: $syncPause.paused });
  const ICONS = { synced: CircleCheck, syncing: RefreshCw, paused: CirclePause, alone: Unplug, offline: CloudOff };

  $: label =
    state.kind === 'syncing'
      ? $t('shell.sync.button.syncing', { n: state.running })
      : $t('shell.sync.button.' + state.kind);
  $: headline =
    state.kind === 'syncing' ? $t('shell.sync.headline.syncing', { n: state.running }) : $t('shell.sync.headline.' + state.kind);
  $: detail =
    state.kind === 'paused'
      ? $t('shell.sync.detail.paused', { length: pauseLength($syncPause, now) })
      : state.kind === 'alone'
        ? $t('shell.sync.detail.alone')
        : state.lastAt
          ? $t('shell.sync.detail.last', { when: said(agoKey(state.lastAt, now)) })
          : $t('shell.sync.detail.never');

  // Each paired device, online ones first, and where it stands.
  $: devices = Object.values($peers)
    .sort((a, b) => Number(b.status === 'online') - Number(a.status === 'online') || a.name.localeCompare(b.name))
    .map((p) => {
      const synced = p.lastSynced ? said(agoKey(Date.parse(p.lastSynced), now)) : '';
      const where = p.status !== 'online' ? 'offline' : p.overRelay ? 'internet' : 'network';
      return {
        id: p.id,
        name: p.name,
        type: p.deviceType,
        online: p.status === 'online',
        line: synced ? $t(`shell.sync.device.${where}Synced`, { when: synced }) : $t(`shell.sync.device.${where}`)
      };
    });

  $: cloudReady = !!$settings?.cloudSync?.ready;
  $: cloudLine = cloudReady
    ? $t('shell.sync.cloudTo', {
        provider: $settings?.cloudSync?.provider && providerById($settings.cloudSync.provider) ? $t('cloud.provider.' + $settings.cloudSync.provider) : $t('home.summary.cloudOn')
      })
    : $t('shell.sync.cloudNotSetUp');

  let button;
  let panel;
  const close = () => syncPanelOpen.set(false);
  function outside(e) {
    if (!$syncPanelOpen) return;
    if (panel?.contains(e.target) || button?.contains(e.target)) return;
    // A menu opened from the panel (Pause) is outside it, and not a reason to close.
    if (e.target.closest?.('.menu')) return;
    close();
  }

  function syncAll() {
    syncAllGames();
    toast($t('home.syncAllToast'));
  }
  const pauseMenu = (e) =>
    openMenu(
      e,
      PAUSE_CHOICES.map((c) => ({ label: `Pause syncing ${c.label}`, icon: Pause, run: () => pauseSync(c.minutes) }))
    );
  function go(page) {
    close();
    navigate(page);
  }
</script>

<svelte:window on:mousedown={outside} on:keydown={(e) => $syncPanelOpen && e.key === 'Escape' && close()} />

<div class="anchor" style="--wails-draggable: no-drag">
  <button
    bind:this={button}
    class="sync-btn kind-{state.kind}"
    class:open={$syncPanelOpen}
    aria-expanded={$syncPanelOpen}
    aria-haspopup="dialog"
    on:click={() => syncPanelOpen.update((o) => !o)}
    title={detail}
  >
    {#key state.kind}
      <span class="icon" class:spin={state.kind === 'syncing'} in:pop={{ start: 0.6, duration: 220 }}><svelte:component this={ICONS[state.kind]} size={15} /></span>
    {/key}
    <span class="label">{#key label}<span class="swap" in:rise={{ y: 4, duration: 180 }}>{label}</span>{/key}</span>
    <span class="chev"><ChevronDown size={13} /></span>
  </button>

  {#if $syncPanelOpen}
    <div class="panel" bind:this={panel} role="dialog" aria-label={$t('shell.sync.title')} transition:unfold>
      <div class="top kind-{state.kind}">
        <span class="state-icon"><svelte:component this={ICONS[state.kind]} size={18} /></span>
        <div class="state-text">
          <p class="headline" role="status">{headline}</p>
          <p class="detail">{detail}</p>
        </div>
      </div>

      <ul class="devices">
        <li style="--i: 0">
          <span class="dtile here"><svelte:component this={isHandheld($settings?.deviceType) ? Gamepad2 : Monitor} size={17} /></span>
          <span class="dtext"><span class="dname">{$settings?.deviceName ?? '…'}</span><span class="dline">{$t('shell.sync.thisDevice', { n: $gameList.length })}</span></span>
        </li>
        {#each devices as d, i (d.id)}
          <li class:offline={!d.online} style="--i: {i + 1}">
            <span class="dtile"><svelte:component this={isHandheld(d.type) ? Gamepad2 : Monitor} size={17} /></span>
            <span class="dtext"><span class="dname">{d.name}</span><span class="dline">{d.line}</span></span>
          </li>
        {/each}
        <li class:offline={!cloudReady} style="--i: {devices.length + 1}">
          <span class="dtile"><Cloud size={17} /></span>
          <span class="dtext"><span class="dname">{$t('shell.sync.cloud')}</span><span class="dline">{cloudLine}</span></span>
          {#if !cloudReady}<button class="btn small ghost" on:click={() => go('cloud')}>{$t('shell.sync.setUp')}</button>{/if}
        </li>
      </ul>

      <div class="actions">
        <button class="btn primary small" on:click={syncAll} disabled={$syncPause.paused || $gameList.length === 0}>
          <RefreshCw size={14} />{$t('shell.sync.syncAll')}
        </button>
        {#if $syncPause.paused}
          <button class="btn small" on:click={resumeSync}><Play size={14} />{$t('statusBar.resume')}</button>
        {:else}
          <button class="btn small" on:click={pauseMenu}><Pause size={14} />{$t('shell.sync.pause')}</button>
        {/if}
        <span class="grow"></span>
        <button class="btn small ghost" on:click={() => go('devices')}>{$t('home.devices')}</button>
      </div>
    </div>
  {/if}
</div>

<style>
  .anchor {
    position: relative;
    display: flex;
    align-items: center;
  }
  .sync-btn {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 30px;
    padding: 0 8px 0 10px;
    border: 1px solid transparent;
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-md);
    font-weight: 600;
    cursor: pointer;
  }
  .sync-btn:hover,
  .sync-btn.open {
    background: var(--bg-hover);
    border-color: var(--border-strong);
  }
  .sync-btn {
    transition:
      background 0.12s,
      border-color 0.12s;
  }
  .chev {
    display: inline-flex;
    color: var(--text-faint);
    transition: transform 0.18s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .open .chev {
    transform: rotate(180deg);
  }
  .label,
  .swap {
    display: inline-block;
  }
  .icon {
    transition: color 0.2s;
  }
  /* Opening, the rows come in one after another. */
  .devices li {
    animation: row-in 0.24s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(40ms + var(--i) * 35ms);
  }
  @keyframes row-in {
    from {
      opacity: 0;
      transform: translateY(4px);
    }
  }
  .icon {
    display: inline-flex;
    color: var(--success);
  }
  .kind-syncing .icon,
  .kind-syncing .state-icon {
    color: var(--accent-text);
  }
  .kind-paused .icon,
  .kind-paused .state-icon,
  .kind-offline .icon,
  .kind-offline .state-icon {
    color: var(--warn);
  }
  .kind-alone .icon,
  .kind-alone .state-icon {
    color: var(--text-dim);
  }
  .spin :global(svg) {
    animation: spin 1.4s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .panel {
    position: absolute;
    top: calc(100% + 8px);
    right: 0;
    z-index: 60;
    width: 360px;
    max-width: calc(100vw - 24px);
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
    overflow: hidden;
  }
  .top {
    display: flex;
    gap: 12px;
    align-items: center;
    padding: 16px;
  }
  .state-icon {
    width: 38px;
    height: 38px;
    border-radius: 11px;
    display: grid;
    place-items: center;
    flex-shrink: 0;
    color: var(--success);
    background: var(--bg-active);
  }
  .headline {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .detail {
    font-size: var(--text-sm);
    color: var(--text-faint);
    margin-top: 1px;
  }
  .devices {
    list-style: none;
    border-top: 1px solid var(--border);
    padding: 6px 8px;
  }
  .devices li {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px;
    border-radius: var(--radius);
  }
  .dtile {
    width: 34px;
    height: 34px;
    border-radius: 9px;
    display: grid;
    place-items: center;
    flex-shrink: 0;
    background: var(--bg-active);
    color: var(--success);
  }
  .dtile.here {
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .offline .dtile {
    color: var(--text-faint);
  }
  .dtext {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .dname {
    font-weight: 600;
    font-size: var(--text-md);
  }
  .dline {
    font-size: var(--text-sm);
    color: var(--text-dim);
  }
  .offline .dline {
    color: var(--text-faint);
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 12px 16px;
    border-top: 1px solid var(--border);
    background: var(--bg-sidebar);
  }
  .grow {
    flex: 1;
  }
  @media (prefers-reduced-motion: reduce) {
    .spin :global(svg) {
      animation: none;
    }
  }
</style>
