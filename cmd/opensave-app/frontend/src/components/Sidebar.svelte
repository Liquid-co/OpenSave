<script>
  // The sidebar: this device, the pages, and the games you starred. Not every
  // game: the Library is the list of everything; this is the few you go back
  // to. Three sizes (lib/shell.js): full, icons only, or put away, chosen
  // with the title bar's button or Ctrl+B. A full one folds to icons in a
  // narrow window, so the page keeps its room.
  //
  // Which games' explicit covers are currently revealed. Held per-game rather
  // than as one "show everything" flag: revealing one cover should not uncover
  // the rest of the shelf. A Set in a plain variable would not re-render —
  // Svelte tracks assignment, so both helpers reassign.
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
  import { slidingIndicator } from '../lib/motion.js';
  import { view, navigate, settings, conflictCount, pairingRequests, syncActivity } from '../lib/stores.js';
  import { gameCover } from '../lib/api.js';
  import CoverImage from './CoverImage.svelte';
  import { visibleGames } from '../lib/gameactions.js';
  import { openGameMenu } from '../lib/contextmenu.js';
  import { collections, FAVOURITES } from '../lib/collections.js';
  import { sidebar, sidebarShown } from '../lib/shell.js';
  import { PAGE_LABEL_KEYS } from '../lib/shortcuts.js';
  import { t } from '../lib/i18n.js';
  import LayoutGrid from 'lucide-svelte/icons/layout-grid';
  import MonitorSmartphone from 'lucide-svelte/icons/monitor-smartphone';
  import Cloud from 'lucide-svelte/icons/cloud';
  import Activity from 'lucide-svelte/icons/activity';
  import Settings from 'lucide-svelte/icons/settings';
  import Sparkles from 'lucide-svelte/icons/sparkles';
  import Star from 'lucide-svelte/icons/star';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';

  // In the order of Ctrl+1 … Ctrl+5 (lib/shortcuts.js PAGES).
  const nav = [
    { id: 'home', icon: LayoutGrid },
    { id: 'activity', icon: Activity },
    { id: 'devices', icon: MonitorSmartphone },
    { id: 'cloud', icon: Cloud },
    { id: 'settings', icon: Settings }
  ];

  let width = globalThis.innerWidth ?? 1440;
  $: shown = sidebarShown($sidebar, width);
  $: full = shown === 'full';

  $: deviceName = $settings?.deviceName ?? '…';
  $: favouriteIds = new Set($collections.find((c) => c.id === FAVOURITES)?.gameIds ?? []);
  $: favourites = $visibleGames.filter((g) => favouriteIds.has(g.id)).sort((a, b) => a.name.localeCompare(b.name));

  function badgeFor(id) {
    if (id === 'devices' && $pairingRequests.length > 0) return $pairingRequests.length;
    if (id === 'home' && $conflictCount > 0) return $conflictCount;
    return 0;
  }

  function initials(name) {
    return name.split(/\s+/).map((w) => w[0]).join('').slice(0, 2).toUpperCase();
  }
</script>

<svelte:window bind:innerWidth={width} />

{#if shown !== 'hidden'}
  <aside class:icons={!full} aria-label={$t('shell.sidebar.label')}>
    <div class="profile" title={full ? '' : `${deviceName}, ${$t('sidebar.thisDevice')}`}>
      <div class="avatar">{initials(deviceName)}</div>
      {#if full}
        <div class="who">
          <div class="name">{deviceName}</div>
          <div class="sub">{$t('sidebar.thisDevice')}</div>
        </div>
      {/if}
    </div>

    <nav use:slidingIndicator={{ mode: 'fill', className: 'slide-nav' }} aria-label={$t('shell.sidebar.pages')}>
      {#each nav as item}
        <button
          class:active={$view.name === item.id}
          on:click={() => navigate(item.id)}
          title={full ? '' : $t(PAGE_LABEL_KEYS[item.id])}
          aria-label={full ? null : $t(PAGE_LABEL_KEYS[item.id])}
          aria-current={$view.name === item.id ? 'page' : null}
        >
          <svelte:component this={item.icon} size={17} strokeWidth={1.8} />
          {#if full}<span>{$t(PAGE_LABEL_KEYS[item.id])}</span>{/if}
          {#if badgeFor(item.id)}
            <span class="nav-badge">{badgeFor(item.id)}</span>
          {/if}
        </button>
      {/each}
    </nav>

    <section class="favourites" aria-label={$t('shell.sidebar.favourites')}>
      {#if full}
        <div class="section-head"><Star size={12} />{$t('shell.sidebar.favourites')}</div>
      {/if}
      <div class="list">
        {#each favourites as game (game.id)}
          <button
            class="game"
            class:active={$view.name === 'game' && $view.params.gameId === game.id}
            class:insession={!!game.playingSince}
            on:click={() => navigate('game', { gameId: game.id })}
            on:contextmenu={(e) => openGameMenu(e, game)}
            on:mouseenter={() => reveal(game)}
            on:mouseleave={() => unreveal(game)}
            on:focus={() => reveal(game)}
            on:blur={() => unreveal(game)}
            title={full ? '' : game.name}
            aria-label={full ? null : game.name}
          >
            <span class="thumb">
              <span class="cover-fallback">{initials(game.name)}</span>
              <!-- Through the daemon, which caches and knows whether the art
                   is explicit (see gameCover in lib/api.js). -->
              <CoverImage src={gameCover(game, true)} alt="" revealed={revealed.has(game.id)} />
            </span>
            {#if full}
              <span class="game-name">{game.name}</span>
              {#if game.playingSince}
                <span class="session" title={$t('sidebar.inSession')}><Gamepad2 size={13} /></span>
              {:else if $syncActivity[game.id]?.state === 'running'}
                <span class="spin" title={$t('sidebar.syncing')}></span>
              {/if}
            {/if}
          </button>
        {:else}
          {#if full}
            <p class="empty">{$t('shell.sidebar.noFavourites')}</p>
          {/if}
        {/each}
      </div>
    </section>

    <div class="foot">
      <button class="whatsnew" class:active={$view.name === 'changelog'} on:click={() => navigate('changelog')} title={full ? '' : $t(PAGE_LABEL_KEYS.changelog)} aria-label={full ? null : $t(PAGE_LABEL_KEYS.changelog)}>
        <Sparkles size={15} strokeWidth={1.8} />
        {#if full}<span>{$t(PAGE_LABEL_KEYS.changelog)}</span>{/if}
      </button>
    </div>
  </aside>
{/if}

<style>
  aside {
    width: var(--sidebar-w);
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 18px;
    flex-shrink: 0;
    min-height: 0;
    padding: 16px 0 10px;
  }
  aside.icons {
    width: var(--sidebar-w-icons);
  }
  .profile {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 0 18px;
  }
  .icons .profile {
    justify-content: center;
    padding: 0;
  }
  .avatar {
    width: 36px;
    height: 36px;
    flex-shrink: 0;
    border-radius: 10px;
    background: var(--accent-soft);
    color: var(--accent-text);
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: var(--text-md);
  }
  .who {
    min-width: 0;
  }
  .who .name {
    font-weight: 600;
    font-size: var(--text-base);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .who .sub {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }

  nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 0 10px;
  }
  nav button,
  .whatsnew {
    position: relative;
    display: flex;
    align-items: center;
    gap: 12px;
    height: 36px;
    padding: 0 12px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-base);
    cursor: pointer;
    text-align: left;
    white-space: nowrap;
  }
  .icons nav button,
  .icons .whatsnew {
    justify-content: center;
    padding: 0;
  }
  nav button:hover,
  .whatsnew:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  nav button.active,
  .whatsnew.active {
    background: var(--bg-active);
    color: var(--text);
    font-weight: 600;
  }
  /* With the selection sliding (slidingIndicator, lib/motion.js), one marker
     draws the active fill and moves between items; the buttons put theirs
     away and sit over it. */
  nav:global(.slides) button {
    z-index: 1;
  }
  nav:global(.slides) button.active {
    background: transparent;
  }
  nav :global(.slide-nav) {
    background: var(--bg-active);
    border-radius: var(--radius-sm);
  }
  nav button.active :global(svg) {
    color: var(--accent-text);
  }
  .nav-badge {
    margin-left: auto;
    background: var(--accent-fill);
    color: var(--on-accent);
    border-radius: 999px;
    font-size: var(--text-xs);
    font-weight: 700;
    padding: 1px 7px;
  }
  .icons .nav-badge {
    position: absolute;
    top: 3px;
    right: 6px;
    padding: 0 5px;
    font-size: 0.65rem;
  }

  .favourites {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .icons .favourites {
    border-top: 1px solid var(--border);
    margin: 0 12px;
    padding-top: 14px;
  }
  .section-head {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 22px 4px;
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-faint);
  }
  .section-head :global(svg) {
    color: var(--warn);
  }
  .list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 0 10px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .icons .list {
    padding: 0;
    align-items: center;
    gap: 6px;
  }
  .game {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    height: 36px;
    padding: 0 10px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-md);
    cursor: pointer;
    text-align: left;
    flex-shrink: 0;
  }
  .icons .game {
    width: 40px;
    height: 48px;
    padding: 0;
    justify-content: center;
  }
  .game:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .game.active {
    background: var(--bg-active);
    color: var(--text);
  }
  /* Its menu is open (lib/contextmenu.js): outlined, so the menu's game is
     plain once the pointer has moved onto the menu. */
  .game:global([data-menu-open]) {
    background: var(--bg-active);
    color: var(--text);
    box-shadow: inset 0 0 0 1px var(--accent);
  }
  .thumb {
    position: relative;
    width: 20px;
    height: 27px;
    flex-shrink: 0;
    overflow: hidden;
    border-radius: 3px;
  }
  .icons .thumb {
    width: 28px;
    height: 38px;
    border-radius: 4px;
  }
  .thumb :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    transition: filter 120ms ease, transform 120ms ease;
  }
  .cover-fallback {
    position: absolute;
    inset: 0;
    background: var(--bg-active);
    color: var(--text-faint);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.6rem;
    font-weight: 700;
  }
  .game-name {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  /* The game being played: its name and a controller, in green. */
  .game.insession .game-name {
    color: var(--success);
  }
  .icons .game.insession .thumb {
    box-shadow: 0 0 0 2px var(--success);
  }
  .session {
    display: inline-flex;
    margin-left: auto;
    flex-shrink: 0;
    color: var(--success);
  }
  .empty {
    padding: 4px 12px;
    color: var(--text-faint);
    font-size: var(--text-sm);
    line-height: 1.45;
  }
  .foot {
    padding: 0 10px;
  }
  .whatsnew {
    width: 100%;
    font-size: var(--text-md);
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
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
