<script>
  // Which games' explicit covers are currently revealed. Held per-game rather
  // than as one "show everything" flag: revealing one cover should not uncover
  // the rest of the shelf.
  //
  // A Set in a plain variable would not re-render — Svelte tracks assignment,
  // so both helpers reassign.
  let revealed = new Set();
  const reveal = (game) => {
    if (game?.coverExplicit && !revealed.has(game.id)) {
      revealed = new Set(revealed).add(game.id);
    }
  };
  const unreveal = (game) => {
    if (game?.coverExplicit && revealed.has(game.id)) {
      const next = new Set(revealed);
      next.delete(game.id);
      revealed = next;
    }
  };
  import { view, navigate, settings, stateLoaded, gameList, conflictCount, pairingRequests, syncActivity } from '../lib/stores.js';
  import { gameCover } from '../lib/api.js';
  import CoverImage from './CoverImage.svelte';
  import House from 'lucide-svelte/icons/house';
  import MonitorSmartphone from 'lucide-svelte/icons/monitor-smartphone';
  import Cloud from 'lucide-svelte/icons/cloud';
  import Activity from 'lucide-svelte/icons/activity';
  import Settings from 'lucide-svelte/icons/settings';
  import ScrollText from 'lucide-svelte/icons/scroll-text';
  import Plus from 'lucide-svelte/icons/plus';
  import Search from 'lucide-svelte/icons/search';

  let filter = '';

  const nav = [
    { id: 'home', label: 'Home', icon: House },
    { id: 'devices', label: 'Devices', icon: MonitorSmartphone },
    { id: 'cloud', label: 'Cloud Backup', icon: Cloud },
    { id: 'activity', label: 'Activity', icon: Activity },
    { id: 'settings', label: 'Settings', icon: Settings },
    { id: 'changelog', label: 'Changelog', icon: ScrollText }
  ];

  $: deviceName = $settings?.deviceName ?? '…';
  $: filteredGames = $gameList.filter((g) => g.name.toLowerCase().includes(filter.toLowerCase()));
  $: activeSyncs = Object.values($syncActivity).filter((s) => s.state === 'running').length;

  function badgeFor(id) {
    if (id === 'devices' && $pairingRequests.length > 0) return $pairingRequests.length;
    if (id === 'home' && $conflictCount > 0) return $conflictCount;
    return 0;
  }

  function initials(name) {
    return name.split(/\s+/).map((w) => w[0]).join('').slice(0, 2).toUpperCase();
  }
</script>

<aside>
  <div class="profile">
    <div class="avatar">{initials(deviceName)}</div>
    <div class="who">
      <div class="name">{deviceName}</div>
      <div class="sub">this device</div>
    </div>
  </div>

  <nav>
    {#each nav as item}
      <button class:active={$view.name === item.id} on:click={() => navigate(item.id)}>
        <svelte:component this={item.icon} size={17} strokeWidth={1.8} />
        <span>{item.label}</span>
        {#if badgeFor(item.id)}
          <span class="nav-badge">{badgeFor(item.id)}</span>
        {/if}
      </button>
    {/each}
  </nav>

  <div class="library-head">
    <span>MY LIBRARY</span>
    <button class="add" title="Track a game" aria-label="Track a game" on:click={() => navigate('home', { add: true })}><Plus size={15} /></button>
  </div>

  <label class="filter">
    <Search size={14} />
    <input placeholder="Filter library" aria-label="Filter library" bind:value={filter} />
  </label>

  <div class="library">
    {#each filteredGames as game (game.id)}
      <button
        class="game"
        class:active={$view.name === 'game' && $view.params.gameId === game.id}
        on:click={() => navigate('game', { gameId: game.id })}
        on:mouseenter={() => reveal(game)}
        on:mouseleave={() => unreveal(game)}
        on:focus={() => reveal(game)}
        on:blur={() => unreveal(game)}
      >
        <span class="thumb">
          <span class="cover-fallback">{initials(game.name)}</span>
          <!--
            Through the daemon, not the stored URL. game.coverUrl is empty for a
            game with no App ID, so this rendered no image at all for exactly the
            titles the name lookup was added to cover; and when it was set it
            pointed straight at Steam's CDN, which the embedded webview cannot
            reliably reach. gameCover asks the local daemon, which caches, falls
            back to an image proxy, and knows whether what it returned is explicit.
          -->
          <CoverImage
            src={gameCover(game)}
            alt=""
            revealed={revealed.has(game.id)}
          />
        </span>
        <span class="game-name">{game.name}</span>
        {#if $syncActivity[game.id]?.state === 'running'}
          <span class="spin" title="Syncing"></span>
        {/if}
      </button>
    {:else}
      {#if $stateLoaded}
        <div class="library-empty">
          {$gameList.length === 0 ? 'No games tracked yet' : 'No matches'}
        </div>
      {/if}
    {/each}
  </div>

  {#if activeSyncs > 0}
    <div class="sync-note">
      <span class="spin"></span>
      {activeSyncs} sync{activeSyncs > 1 ? 's' : ''} in progress
    </div>
  {/if}
</aside>

<style>
  .thumb {
    overflow: hidden;
    border-radius: 6px;
  }
  .thumb :global(img) {
    transition: filter 120ms ease, transform 120ms ease;
  }
  aside {
    width: var(--sidebar-w);
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    min-height: 0;
  }
  .profile {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 16px 16px 12px;
  }
  .avatar {
    width: 38px;
    height: 38px;
    border-radius: 10px;
    background: var(--accent-soft);
    color: var(--accent);
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 0.9rem;
  }
  .who .name {
    font-weight: 600;
    font-size: 0.95rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 150px;
  }
  .who .sub {
    font-size: 0.72rem;
    color: var(--text-faint);
  }

  nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 4px 10px;
  }
  nav button {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 9px 12px;
    border: none;
    border-radius: var(--radius);
    background: transparent;
    color: var(--text-dim);
    font-size: 0.93rem;
    font-weight: 500;
    cursor: pointer;
    text-align: left;
  }
  nav button:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  nav button {
    position: relative;
  }
  nav button.active {
    background: var(--bg-active);
    color: var(--text);
  }
  nav button.active::before {
    content: '';
    position: absolute;
    left: -10px;
    top: 9px;
    bottom: 9px;
    width: 3px;
    border-radius: 0 3px 3px 0;
    background: var(--accent);
  }
  nav button.active :global(svg) {
    color: var(--accent);
  }
  .nav-badge {
    margin-left: auto;
    background: var(--accent);
    color: #fff;
    border-radius: 999px;
    font-size: 0.7rem;
    font-weight: 700;
    padding: 1px 7px;
  }

  .library-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 18px 8px;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    color: var(--text-faint);
  }
  .library-head .add {
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    border: none;
    background: transparent;
    color: var(--text-dim);
    cursor: pointer;
    border-radius: 6px;
  }
  .library-head .add:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .filter {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 14px 8px;
    padding: 0 11px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--text-faint);
    cursor: text;
  }
  .filter:focus-within {
    border-color: var(--border-strong);
  }
  .filter input {
    flex: 1;
    min-width: 0;
    padding: 7px 0;
    border: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.85rem;
    outline: none;
  }

  .library {
    flex: 1;
    overflow-y: auto;
    padding: 0 10px 10px;
    min-height: 0;
  }
  .game {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 6px 8px;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--text-dim);
    font-size: 0.88rem;
    cursor: pointer;
    text-align: left;
  }
  .game:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .game.active {
    background: var(--bg-active);
    color: var(--text);
  }
  .thumb {
    position: relative;
    width: 24px;
    height: 24px;
    flex-shrink: 0;
  }
  .thumb :global(img) {
    position: absolute;
    inset: 0;
    width: 24px;
    height: 24px;
    border-radius: 6px;
    object-fit: cover;
  }
  .cover-fallback {
    position: absolute;
    inset: 0;
    border-radius: 6px;
    background: var(--bg-active);
    color: var(--text-faint);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.62rem;
    font-weight: 700;
  }
  .game-name {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .library-empty {
    padding: 18px 10px;
    color: var(--text-faint);
    font-size: 0.82rem;
    text-align: center;
  }

  .sync-note {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 18px;
    border-top: 1px solid var(--border);
    color: var(--text-dim);
    font-size: 0.8rem;
  }
  .spin {
    width: 11px;
    height: 11px;
    border: 2px solid var(--accent-soft);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    flex-shrink: 0;
    margin-left: auto;
  }
  .sync-note .spin {
    margin-left: 0;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
