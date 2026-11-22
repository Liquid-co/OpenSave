<script>
  import { onMount, onDestroy } from 'svelte';
  import { initApi, connectWS, native } from './lib/api.js';
  import { applyMessage, wsConnected, view, toast, showAbout, cloudOffers, newGames, navigate, settings, games, availableUpdate } from './lib/stores.js';
  import { justUpdated, updateNotice } from './lib/update.js';
  import { startController, controllerOn, padUsed, pageStep } from './lib/controller.js';
  import { startFocusRing } from './lib/focusring.js';
  import { appearance } from './lib/appearance.js';
  import { locale, resolveLocale, t } from './lib/i18n.js';
  import { paletteOpen } from './lib/shortcuts.js';
  import { newlyEmptied } from './lib/emptied.js';
  import { demandAttention } from './lib/notify.js';

  import logoUrl from './assets/logo.png';
  import TitleBar from './components/TitleBar.svelte';
  import Sidebar from './components/Sidebar.svelte';
  import StatusBar from './components/StatusBar.svelte';
  import Toasts from './components/Toasts.svelte';
  import ConflictModal from './components/ConflictModal.svelte';
  import LocationConflictModal from './components/LocationConflictModal.svelte';
  import PairingBanner from './components/PairingBanner.svelte';
  import CloudOfferBanner from './components/CloudOfferBanner.svelte';
  import NewGamesBanner from './components/NewGamesBanner.svelte';
  import UpdateNotice from './components/UpdateNotice.svelte';
  import ConfirmDialog from './components/ConfirmDialog.svelte';
  import ContextMenu from './components/ContextMenu.svelte';
  import Shortcuts from './components/Shortcuts.svelte';
  import RestoreDialog from './components/RestoreDialog.svelte';
  import DropOverlay from './components/DropOverlay.svelte';
  import CollectionsDialog from './components/CollectionsDialog.svelte';

  import Home from './views/Home.svelte';
  import GameDetail from './views/GameDetail.svelte';
  import Devices from './views/Devices.svelte';
  import Backups from './views/Backups.svelte';
  import SettingsMenu from './components/SettingsMenu.svelte';
  import { settingsMenu } from './lib/settingsmenu.js';
  import Changelog from './views/Changelog.svelte';
  import ActivityLog from './views/ActivityLog.svelte';

  let ready = false;
  let bootError = '';
  let retrying = false;

  async function boot() {
    bootError = '';
    retrying = true;
    try {
      await initApi();
      connectWS(applyMessage, (up) => wsConnected.set(up));
      ready = true;
    } catch (e) {
      bootError = e.message;
    }
    retrying = false;
  }

  // Moving around with a controller, or the arrow keys the same way, when
  // that is on (lib/controller.js). The focus ring is drawn plainly then:
  // with no pointer, it is the only way to see where you are.
  $: controllerActive = controllerOn($appearance.controller, { deviceType: $settings?.deviceType, used: $padUsed });
  $: document.documentElement.dataset.controller = controllerActive ? 'on' : 'off';
  // The document language follows the chosen locale ('system' resolved); a
  // wrong lang makes Windows render some Han characters with Japanese shapes.
  $: document.documentElement.lang = resolveLocale($locale);
  // The highlight on what the keyboard or the controller has selected.
  onMount(() => startFocusRing());
  onMount(() =>
    startController({
      isOn: () => controllerActive,
      actions: {
        back: () => $view.name !== 'home' && navigate('home'),
        menu: (el) => el?.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true })),
        palette: () => paletteOpen.set(true),
        page: (delta) => navigate(pageStep($view.name, delta))
      }
    })
  );

  // A save emptied here waits on an answer before anything syncs: said once,
  // loudly, when it starts waiting (lib/emptied.js).
  let emptiedBefore = null;
  const stopEmptied = games.subscribe((all) => {
    const { now, fresh } = newlyEmptied(emptiedBefore, all);
    emptiedBefore = now;
    for (const g of fresh) {
      toast($t('app.emptiedBody', { name: g.name }), 'warning', {
        ttl: 15000,
        action: { label: $t('app.choose'), run: () => navigate('game', { gameId: g.id }) }
      });
      demandAttention('emptied', {
        title: g.name,
        body: $t('app.emptiedBody', { name: g.name }),
        game: g
      });
    }
  });
  onDestroy(stopEmptied);

  onMount(async () => {
    // The tray's "Open Activity" and the like: the desktop shell asks for a
    // page by name. Absent in a browser.
    globalThis.runtime?.EventsOn?.('navigate', (page) => navigate(page));
    // A desktop notification clicked: where it was about (lib/notify.js).
    globalThis.runtime?.EventsOn?.('notification-open', (target) => {
      try {
        const { view, params } = JSON.parse(target);
        if (view) navigate(view, params ?? {});
      } catch {
        // Not one of ours.
      }
    });
    await boot();
    // First launch after an update (including peer-to-peer, which carries
    // no release notes): announce it and offer the embedded changelog.
    try {
      const g = await native.updateGreeting();
      if (g?.updatedFrom) {
        const info = await native.appInfo();
        if (info?.version) justUpdated.set({ to: info.version, from: g.updatedFrom });
      }
    } catch {}
    // Non-blocking: never let an update check affect startup. Re-check
    // every 6h too — the app lives in the tray for weeks at a time.
    const check = async () => {
      try {
        const res = await native.checkUpdate();
        if (res?.available) availableUpdate.set(res);
      } catch {}
    };
    await check();
    setInterval(check, 6 * 3600 * 1000);
  });

  const views = {
    home: Home,
    game: GameDetail,
    devices: Devices,
    // 'internet' is kept as an alias so any deep-link opens Devices on its
    // Over-the-internet tab (Internet Sync now lives there).
    internet: Devices,
    cloud: Backups,
    changelog: Changelog,
    activity: ActivityLog
  };

  // A new page opens at its top. The page area is one scroller shared by every
  // page, so without this the next page opened as far down as the last had
  // been scrolled — Activity half-way down because Settings was. Only a change
  // of page: something that changes within one (a dialog it opens) keeps its
  // place.
  let mainEl;
  let shownPage = '';
  // How much room the notices above the page take, for the cover behind the
  // Library and a game's header (LibraryBackdrop) to reach up behind them —
  // otherwise it began below them, with a hard edge under the card.
  let noticesHeight = 0;
  $: noticesShown = !!$updateNotice || $cloudOffers.length > 0 || $newGames.length > 0;
  $: noticesSpace = noticesShown ? noticesHeight + 18 : 0;
  $: if (mainEl && $view.name !== shownPage) {
    if (shownPage) mainEl.scrollTop = 0;
    shownPage = $view.name;
  }
</script>

<div class="shell">
  <TitleBar />
  <div class="body">
    {#if bootError}
      <div class="boot-error">
        <img class="boot-logo" src={logoUrl} alt="OpenSave" />
        <h2>{$t('app.failedToStart')}</h2>
        <p>{bootError}</p>
        <button class="btn primary" disabled={retrying} on:click={boot}>
          {retrying ? $t('app.retrying') : $t('app.retry')}
        </button>
        <p class="boot-hint">
          {$t('app.detailsSaved')}
        </p>
      </div>
    {:else if ready}
      <Sidebar />
      <!-- The page and its toasts: they sit at the bottom centre of the page,
           above the status bar and never over the sidebar. -->
      <div class="page">
      <main bind:this={mainEl} style="--notices: {noticesSpace}px">
        <!-- At the top of the page rather than floating over it: floating, they
             sat on the page's own header buttons for as long as they were
             shown. These wait for an answer; they should not take anything away
             while they do. -->
        {#if noticesShown}
          <div class="notices" bind:offsetHeight={noticesHeight}>
            <UpdateNotice />
            <CloudOfferBanner />
            <NewGamesBanner />
          </div>
        {/if}
        <!-- Keyed by page, so arriving somewhere new plays the short rise in
             app.css (still, with animations off); moving between two games
             is the same page and does not. The page being left goes at once
             and the next is there at once: nothing waits on an animation to
             be seen, so a click answers even when frames come slowly. -->
        <div class="views">
          {#key $view.name}
            <div class="view">
              <svelte:component this={views[$view.name] ?? Home} params={$view.params} />
            </div>
          {/key}
        </div>
      </main>
      <Toasts />
      </div>
    {:else}
      <div class="boot-loading">
        <img class="boot-logo pulse" src={logoUrl} alt="OpenSave" />
        <span>{$t('app.startingOpenSave')}</span>
      </div>
    {/if}
  </div>
  <StatusBar />
  {#if !ready || bootError}<Toasts floating />{/if}
  <ConflictModal />
  <LocationConflictModal />
  <ConfirmDialog />
  <ContextMenu />
  <Shortcuts />
  <RestoreDialog />
  <DropOverlay />
  <CollectionsDialog />
  {#if ready && $settingsMenu}<SettingsMenu />{/if}
  {#if ready}<PairingBanner />{/if}
</div>

<style>
  /* Above the page's cover art, which reaches up behind them. */
  .notices {
    position: relative;
    z-index: 1;
    display: flex;
    flex-direction: column;
    gap: 8px;
    /* As wide as the page below it. A narrower card, at the left edge or
       centred, did not line up with anything on a wide window. */
    margin-bottom: 18px;
  }
  .shell {
    display: flex;
    flex-direction: column;
    height: 100%;
  }
  .body {
    flex: 1;
    display: flex;
    min-height: 0;
  }
  .page {
    position: relative;
    flex: 1;
    min-width: 0;
    display: flex;
  }
  main {
    flex: 1;
    overflow-y: auto;
    padding: 24px 28px;
    min-width: 0;
  }
  .views {
    display: grid;
  }
  .view {
    grid-area: 1 / 1;
    min-width: 0;
    animation: arrive 0.2s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
  }
  .boot-loading,
  .boot-error {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    color: var(--text-dim);
  }
  .boot-error p {
    color: var(--danger);
    max-width: 480px;
    text-align: center;
  }
  .boot-error .btn {
    margin-top: 6px;
  }
  .boot-hint {
    color: var(--text-dim) !important;
    font-size: 0.8rem;
  }
  .boot-logo {
    width: 72px;
    height: 72px;
    border-radius: calc(18px * var(--round, 1));
    margin-bottom: 6px;
  }
  .pulse {
    animation: pulse 1.6s ease-in-out infinite;
  }
  @keyframes pulse {
    0%, 100% { opacity: 0.45; transform: scale(0.97); }
    50% { opacity: 1; transform: scale(1); }
  }
</style>
