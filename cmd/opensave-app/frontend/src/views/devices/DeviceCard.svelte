<script>
  // One paired device: where it is and how it is reached, a problem with the
  // pairing when there is one, the games the two of you share, what moved
  // between you lately, and whether it may choose folders here.
  //
  // Motion: while a save is moving between you, a line says which, its
  // arrows running the way it goes, over a bar of how far it has come. Where
  // the device is, when you last synced and what moved lately come in again
  // when they change.
  import { createEventDispatcher, onDestroy } from 'svelte';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Monitor from 'lucide-svelte/icons/monitor';
  import MoreHorizontal from 'lucide-svelte/icons/ellipsis';
  import LockOpen from 'lucide-svelte/icons/lock-open';
  import ArrowUp from 'lucide-svelte/icons/arrow-up';
  import ArrowDown from 'lucide-svelte/icons/arrow-down';
  import Download from 'lucide-svelte/icons/download';
  import Unlink from 'lucide-svelte/icons/unlink';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import CoverImage from '../../components/CoverImage.svelte';
  import Dropdown from '../../components/ui/Dropdown.svelte';
  import { gameCover } from '../../lib/api.js';
  import { games, navigate, appUpdate, syncActivity } from '../../lib/stores.js';
  import { rise, collapse, gatedFlip } from '../../lib/motion.js';
  import { isHandheld } from '../../lib/devices.js';
  import { protectionState } from '../../lib/protection.js';
  import { peerWhere, gamesOnBoth, recentWith, isOlder, syncingWith } from '../../lib/devicesview.js';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { t } from '../../lib/i18n.js';

  export let peer;
  /** The activity report's items, for what moved lately. */
  export let items = [];
  export let myVersion = '';
  export let busy = false;

  const dispatch = createEventDispatcher();

  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 60_000);
  onDestroy(() => clearInterval(tick));
  const ago = (at) => {
    if (!at) return $t('devices.never');
    const p = timeAgoParts(typeof at === 'number' ? new Date(at).toISOString() : at, now);
    if (p.kind === 'invalid') return p.text;
    const { kind, ...params } = p;
    return $t('timeAgo.' + kind, params);
  };

  $: where = peerWhere(peer);
  $: shared = gamesOnBoth($games, peer.id);
  $: lately = recentWith(items, peer.name, 2);
  $: older = isOlder(peer.appVersion, myVersion);
  $: open = protectionState(peer) === 'open';
  $: SHOWN = 6;
  $: live = syncingWith($syncActivity, peer.name);
  $: first = live[0];
  $: liveName = first ? ($games[first.gameId]?.name ?? first.gameId) : '';

  let menuOpen = false;
  let anchor;
  const outside = (e) => {
    if (menuOpen && !anchor?.contains(e.target)) menuOpen = false;
  };
  $: menu = [
    ...(peer.hasNewerBuild && peer.status === 'online' && !$appUpdate
      ? [{ label: $t('devices.updateFromThis'), icon: Download, run: () => dispatch('update') }]
      : []),
    ...(open ? [{ label: $t('devices.card.pairAgain'), icon: RefreshCw, run: () => dispatch('repair') }] : []),
    { label: $t('devices.unpair'), icon: Unlink, danger: true, run: () => dispatch('unpair') }
  ];
</script>

<svelte:window on:mousedown={outside} />

<article class="card" class:problem={open} class:offline={where === 'offline'} class:live={!!first}>
  <header>
    <span class="tile where-{where}"><svelte:component this={isHandheld(peer.deviceType) ? Gamepad2 : Monitor} size={21} /></span>
    <div class="who">
      <h3>{peer.name}</h3>
      {#key where}<p class="state where-{where}" in:rise={{ y: 4 }}>{$t('devices.card.' + where)}</p>{/key}
    </div>
    <div class="anchor" bind:this={anchor}>
      <button class="more" class:on={menuOpen} aria-label={$t('devices.card.more', { name: peer.name })} aria-haspopup="menu" aria-expanded={menuOpen} on:click={() => (menuOpen = !menuOpen)}>
        <MoreHorizontal size={18} />
      </button>
      <Dropdown items={menu} bind:open={menuOpen} label={$t('devices.card.more', { name: peer.name })} />
    </div>
  </header>

  {#if first}
    <div class="live-row" transition:collapse>
      <span class="flow" class:down={first.direction === 'download'} aria-hidden="true">
        <span class="lane"><svelte:component this={first.direction === 'download' ? ArrowDown : ArrowUp} size={13} /><svelte:component
            this={first.direction === 'download' ? ArrowDown : ArrowUp}
            size={13}
          /></span
        >
      </span>
      <span class="live-what">{live.length > 1 ? $t('devices.card.syncingMore', { name: liveName, n: live.length - 1 }) : $t('devices.card.syncing', { name: liveName })}</span>
      {#if first.percentage}<span class="pct">{first.percentage}%</span>{/if}
      <span class="track"><span class="fill" class:unknown={!first.percentage} style={first.percentage ? `width: ${first.percentage}%` : ''}></span></span>
    </div>
  {/if}

  {#if open}
    <div class="alert">
      <LockOpen size={16} />
      <span>{$t('devices.card.notEncrypted')}</span>
      <button class="btn small" disabled={busy} on:click={() => dispatch('repair')}>{$t('devices.card.pairAgain')}</button>
    </div>
  {/if}

  <dl class="facts">
    <div><dt>{$t('devices.card.lastSynced')}</dt>{#key peer.lastSynced}<dd in:rise={{ y: 4 }}>{ago(peer.lastSynced)}</dd>{/key}</div>
    <div><dt>{$t('devices.card.connection')}</dt><dd>{peer.address === 'relay' ? $t('devices.card.viaInternet') : $t('devices.card.direct')}</dd></div>
    <div><dt>OpenSave</dt><dd class:warn={older} title={older ? $t('devices.card.olderTitle') : ''}>{peer.appVersion || '—'}{#if older}<span class="tag">{$t('devices.card.older')}</span>{/if}</dd></div>
  </dl>

  <section class="sub">
    <h4>{shared.length ? $t('devices.card.gamesOnBoth', { n: shared.length }) : $t('devices.card.noGamesYet')}</h4>
    {#if shared.length}
      <div class="covers">
        {#each shared.slice(0, SHOWN) as g (g.id)}
          <button class="cover" title={g.name} on:click={() => navigate('game', { gameId: g.id })}>
            <CoverImage src={gameCover(g, true)} alt={g.name} />
          </button>
        {/each}
        {#if shared.length > SHOWN}<span class="more-covers">+{shared.length - SHOWN}</span>{/if}
      </div>
    {/if}
  </section>

  {#if lately.length}
    <section class="sub">
      <h4>{$t('devices.card.lately')}</h4>
      {#each lately as it (it.kind + it.gameId + it.atMs)}
        <div class="lately" in:rise={{ y: 4 }} animate:gatedFlip>
          <svelte:component this={it.kind === 'sent' ? ArrowUp : ArrowDown} size={14} />
          <span class="what">{$t(it.kind === 'sent' ? 'devices.card.sent' : 'devices.card.got', { name: $games[it.gameId]?.name ?? it.gameId })}</span>
          <span class="when">{ago(it.atMs)}</span>
        </div>
      {/each}
    </section>
  {/if}

  <footer>
    <label class="check trust" title={$t('devices.trustTooltip')}>
      <input type="checkbox" checked={!!peer.choosesFolders} disabled={busy} on:change={(e) => dispatch('trust', e.currentTarget)} />
      {$t('devices.card.trust')}
    </label>
  </footer>
</article>

<style>
  .card {
    display: flex;
    flex-direction: column;
    padding: 0;
    overflow: visible;
    transition:
      border-color 0.15s,
      transform 0.16s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .card.problem {
    border-color: rgba(var(--warn-rgb), 0.4);
  }
  header {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 12px 12px 16px;
  }
  .tile {
    width: 44px;
    height: 44px;
    border-radius: 12px;
    display: grid;
    place-items: center;
    flex-shrink: 0;
    background: var(--bg-active);
    color: var(--text-faint);
  }
  .tile.where-network,
  .tile.where-internet {
    color: var(--success);
  }
  .tile {
    transition:
      background 0.3s,
      color 0.3s;
  }
  .live .tile {
    background: rgba(var(--accent-rgb), 0.14);
    color: var(--accent-text);
  }

  /* A save on the move: its arrows run the way it goes, on a lane, over a
     bar of how far it has come (or a sweep, until that is known). */
  .live-row {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    column-gap: 8px;
    row-gap: 6px;
    margin: 0 12px 12px;
    padding: 9px 12px 10px;
    border-radius: 10px;
    background: rgba(var(--accent-rgb), 0.08);
    font-size: var(--text-md);
  }
  .flow {
    width: 13px;
    height: 13px;
    overflow: hidden;
    color: var(--accent-text);
  }
  .lane {
    display: flex;
    flex-direction: column;
    animation: run-up 0.9s linear infinite;
  }
  .lane :global(svg) {
    display: block;
    flex: none;
  }
  .down .lane {
    animation-name: run-down;
  }
  @keyframes run-up {
    from {
      transform: translateY(0);
    }
    to {
      transform: translateY(-13px);
    }
  }
  @keyframes run-down {
    from {
      transform: translateY(-13px);
    }
    to {
      transform: translateY(0);
    }
  }
  .live-what {
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .pct {
    color: var(--text-dim);
    font-variant-numeric: tabular-nums;
  }
  .track {
    grid-column: 1 / -1;
    height: 3px;
    border-radius: 2px;
    background: rgba(var(--accent-rgb), 0.16);
    overflow: hidden;
  }
  .fill {
    display: block;
    height: 100%;
    border-radius: 2px;
    background: var(--accent-text);
    transition: width 0.4s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .fill.unknown {
    width: 30%;
    animation: sweep 1.3s cubic-bezier(0.45, 0, 0.55, 1) infinite;
  }
  @keyframes sweep {
    from {
      transform: translateX(-100%);
    }
    to {
      transform: translateX(340%);
    }
  }
  .who {
    flex: 1;
    min-width: 0;
  }
  h3 {
    font-size: var(--text-lg);
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .state {
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .state.where-network,
  .state.where-internet {
    color: var(--success);
  }
  .anchor {
    position: relative;
  }
  .more {
    width: 32px;
    height: 32px;
    display: grid;
    place-items: center;
    border: none;
    border-radius: 9px;
    background: transparent;
    color: var(--text-dim);
    cursor: pointer;
  }
  .more:hover,
  .more.on {
    background: var(--bg-hover);
    color: var(--text);
  }
  .alert {
    display: flex;
    align-items: center;
    gap: 10px;
    margin: 0 12px 12px;
    padding: 9px 10px 9px 12px;
    border-radius: 10px;
    background: rgba(var(--warn-rgb), 0.1);
    color: var(--text);
    font-size: var(--text-md);
  }
  .alert :global(svg) {
    color: var(--warn);
    flex-shrink: 0;
  }
  .alert span {
    flex: 1;
  }
  .facts {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 8px;
    margin: 0;
    padding: 10px 16px;
    border-top: 1px solid var(--border);
  }
  dt {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  dd {
    margin: 0;
    font-size: var(--text-md);
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  dd.warn {
    color: var(--warn);
  }
  .tag {
    margin-left: 6px;
    font-size: var(--text-xs);
    font-weight: 500;
  }
  .sub {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 10px 16px;
    border-top: 1px solid var(--border);
  }
  h4 {
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-faint);
  }
  .covers {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .cover {
    position: relative;
    width: 28px;
    height: 38px;
    padding: 0;
    border: none;
    border-radius: 4px;
    overflow: hidden;
    background: var(--bg-active);
    cursor: pointer;
    transition: transform 0.16s cubic-bezier(0.2, 0.7, 0.2, 1.4);
  }
  .cover:hover {
    transform: translateY(-2px);
  }
  .cover :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .more-covers {
    font-size: var(--text-sm);
    color: var(--text-faint);
    margin-left: 2px;
  }
  .lately {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: var(--text-md);
  }
  .lately :global(svg) {
    color: var(--text-dim);
    flex-shrink: 0;
  }
  .what {
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .when {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  footer {
    margin-top: auto;
    padding: 8px 16px;
    border-top: 1px solid var(--border);
  }
  .trust {
    font-size: var(--text-md);
  }
</style>
