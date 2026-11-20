<script>
  // The window's own title bar (the window has no frame of the system's).
  // Left to right: the sidebar's button, the name, a search that opens the
  // Ctrl+K palette, where syncing stands (SyncPanel), the bell, and the
  // window's buttons. Empty stretches of it move the window; a double-click
  // there maximises it.
  import { native } from '../lib/api.js';
  import { t } from '../lib/i18n.js';
  import { paletteOpen } from '../lib/shortcuts.js';
  import { sidebar, pressSidebarButton, chooseSidebar } from '../lib/shell.js';
  import { openMenu } from '../lib/contextmenu.js';
  import NotificationHub from './NotificationHub.svelte';
  import SyncPanel from './SyncPanel.svelte';
  import PanelLeft from 'lucide-svelte/icons/panel-left';
  import PanelLeftClose from 'lucide-svelte/icons/panel-left-close';
  import Search from 'lucide-svelte/icons/search';
  import logoUrl from '../assets/logo.png';

  const modKey = /Mac/i.test(globalThis.navigator?.platform ?? '') ? '⌘' : 'Ctrl';

  // A double-click on a button is two clicks on it, not a maximise.
  function maximiseFromBar(e) {
    if (e.target.closest?.('button, input, a, .panel')) return;
    native.toggleMaximise();
  }

  // Right-click on the sidebar button: all three sizes by name.
  const sidebarMenu = (e) => {
    e.preventDefault();
    openMenu(e, [
      { label: $t('shell.sidebar.full'), run: () => sidebar.update((s) => chooseSidebar(s, 'full')) },
      { label: $t('shell.sidebar.icons'), run: () => sidebar.update((s) => chooseSidebar(s, 'icons')) },
      { label: $t('shell.sidebar.hidden'), run: () => sidebar.update((s) => chooseSidebar(s, 'hidden')) }
    ]);
  };
</script>

<div class="titlebar" role="banner" style="--wails-draggable: drag" on:dblclick={maximiseFromBar}>
  <div class="lead">
    <button
      class="tog"
      class:on={$sidebar.mode !== 'hidden'}
      style="--wails-draggable: no-drag"
      on:click={() => sidebar.update(pressSidebarButton)}
      on:contextmenu={sidebarMenu}
      title={$t('shell.sidebar.button', { key: modKey })}
      aria-label={$t('shell.sidebar.button', { key: modKey })}
      aria-pressed={$sidebar.mode !== 'hidden'}
    >
      <svelte:component this={$sidebar.mode === 'full' ? PanelLeftClose : PanelLeft} size={16} />
    </button>
    <div class="brand">
      <img class="logo" src={logoUrl} alt="" draggable="false" />
      <span class="title">OpenSave</span>
    </div>
  </div>

  <div class="middle">
    <button class="search" style="--wails-draggable: no-drag" on:click={() => paletteOpen.set(true)} aria-label={$t('shell.search')}>
      <Search size={14} />
      <span class="placeholder">{$t('shell.search')}</span>
      <kbd>{modKey} K</kbd>
    </button>
  </div>

  <div class="right">
    <SyncPanel />
    <NotificationHub />
    <div class="controls" style="--wails-draggable: no-drag">
      <button on:click={() => native.minimise()} title={$t('titleBar.minimise')} aria-label={$t('titleBar.minimise')}>
        <svg width="10" height="10" viewBox="0 0 10 10"><line x1="1" y1="5.5" x2="9" y2="5.5" stroke="currentColor" stroke-width="1.2" /></svg>
      </button>
      <button on:click={() => native.toggleMaximise()} title={$t('titleBar.maximise')} aria-label={$t('titleBar.maximise')}>
        <svg width="10" height="10" viewBox="0 0 10 10"><rect x="1.5" y="1.5" width="7" height="7" fill="none" stroke="currentColor" stroke-width="1.2" /></svg>
      </button>
      <button class="close" on:click={() => native.close()} title={$t('common.close')} aria-label={$t('common.close')}>
        <svg width="10" height="10" viewBox="0 0 10 10"><path d="M1.5 1.5 L8.5 8.5 M8.5 1.5 L1.5 8.5" stroke="currentColor" stroke-width="1.2" /></svg>
      </button>
    </div>
  </div>
</div>

<style>
  .titlebar {
    height: var(--titlebar-h);
    display: flex;
    align-items: center;
    gap: 12px;
    background: var(--bg-sidebar);
    border-bottom: 1px solid var(--border);
    padding-left: 8px;
    flex-shrink: 0;
    position: relative;
    z-index: 40;
  }
  .lead {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 0 1 calc(var(--sidebar-w) - 20px);
    min-width: 0;
  }
  .tog {
    width: 32px;
    height: 30px;
    flex-shrink: 0;
    display: grid;
    place-items: center;
    border: 1px solid transparent;
    border-radius: 8px;
    background: transparent;
    color: var(--text-dim);
    cursor: pointer;
  }
  .tog:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .logo {
    width: 20px;
    height: 20px;
    border-radius: 5px;
  }
  .title {
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
  }
  .middle {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    justify-content: center;
  }
  .search {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    max-width: 480px;
    height: 30px;
    padding: 0 6px 0 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg-raised);
    color: var(--text-faint);
    font: inherit;
    font-size: var(--text-md);
    cursor: text;
    text-align: left;
  }
  .search:hover {
    border-color: var(--border-strong);
    color: var(--text-dim);
  }
  .placeholder {
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  kbd {
    font: inherit;
    font-size: var(--text-xs);
    padding: 1px 6px;
    border: 1px solid var(--border-strong);
    border-radius: 5px;
    color: var(--text-faint);
    flex-shrink: 0;
  }
  .right {
    display: flex;
    align-items: center;
    gap: 4px;
    height: 100%;
    flex-shrink: 0;
  }
  .controls {
    display: flex;
    height: 100%;
    margin-left: 4px;
  }
  .controls button {
    width: 46px;
    height: 100%;
    border: none;
    background: transparent;
    color: var(--text-dim);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .controls button:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .controls button.close:hover {
    background: var(--danger);
    color: #fff;
  }
  /* Narrow windows: the search keeps its icon and its keys. */
  @media (max-width: 900px) {
    .placeholder {
      display: none;
    }
    .search {
      width: auto;
    }
  }
</style>
