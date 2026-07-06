<script>
  import { onMount } from 'svelte';
  import { initApi, connectWS } from './lib/api.js';
  import { applyMessage, wsConnected, view } from './lib/stores.js';

  import logoUrl from './assets/logo.svg';
  import TitleBar from './components/TitleBar.svelte';
  import Sidebar from './components/Sidebar.svelte';
  import StatusBar from './components/StatusBar.svelte';
  import Toasts from './components/Toasts.svelte';
  import ConflictModal from './components/ConflictModal.svelte';

  import Home from './views/Home.svelte';
  import GameDetail from './views/GameDetail.svelte';
  import Devices from './views/Devices.svelte';
  import InternetSync from './views/InternetSync.svelte';
  import CloudBackup from './views/CloudBackup.svelte';
  import Settings from './views/Settings.svelte';
  import ActivityLog from './views/ActivityLog.svelte';

  let ready = false;
  let bootError = '';

  onMount(async () => {
    try {
      await initApi();
      connectWS(applyMessage, (up) => wsConnected.set(up));
      ready = true;
    } catch (e) {
      bootError = e.message;
    }
  });

  const views = {
    home: Home,
    game: GameDetail,
    devices: Devices,
    internet: InternetSync,
    cloud: CloudBackup,
    settings: Settings,
    activity: ActivityLog
  };
</script>

<div class="shell">
  <TitleBar />
  <div class="body">
    {#if bootError}
      <div class="boot-error">
        <img class="boot-logo" src={logoUrl} alt="OpenSave" />
        <h2>OpenSave failed to start</h2>
        <p>{bootError}</p>
      </div>
    {:else if ready}
      <Sidebar />
      <main>
        <svelte:component this={views[$view.name] ?? Home} params={$view.params} />
      </main>
    {:else}
      <div class="boot-loading">
        <img class="boot-logo pulse" src={logoUrl} alt="OpenSave" />
        <span>Starting OpenSave…</span>
      </div>
    {/if}
  </div>
  <StatusBar />
  <Toasts />
  <ConflictModal />
</div>

<style>
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
  main {
    flex: 1;
    overflow-y: auto;
    padding: 24px 28px;
    min-width: 0;
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
  .boot-logo {
    width: 72px;
    height: 72px;
    border-radius: 18px;
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
