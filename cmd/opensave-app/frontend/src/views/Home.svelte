<script>
  import { gameList, peers, syncActivity, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import OfferedGames from './home/OfferedGames.svelte';
  import AddGameCard from './home/AddGameCard.svelte';
  import ScanDialog from './home/ScanDialog.svelte';
  import LibraryGrid from './home/LibraryGrid.svelte';

  export let params = {};

  let scanner;
  let scanning = false;

  let showAdd = params.add ?? false;
  $: if (params.add) showAdd = true;
  // Opened from the new-games card. A token rather than a flag: this
  // statement re-runs whenever the scan closes, and a flag still set would
  // start the scan all over again. Waits for the dialog to exist, which it
  // does not on the first pass.
  let handledScan = null;
  $: if (scanner && params.scan && params.scan !== handledScan) {
    handledScan = params.scan;
    scanner.start();
  }

  async function syncAll() {
    for (const g of $gameList) {
      api.post(`/api/games/${g.id}/sync`).catch(() => {});
    }
    toast('Sync triggered for all games');
  }

  $: onlinePeers = Object.values($peers).filter((p) => p.status === 'online');
</script>

<div class="head">
  <h2 class="page-title">Home</h2>
  <div class="head-actions">
    <button class="btn" on:click={() => scanner.start()} disabled={scanning}>
      {scanning ? 'Scanning…' : '🔍 Auto-scan'}
    </button>
    <button class="btn" on:click={syncAll} disabled={$gameList.length === 0}>⟳ Sync all</button>
    <button class="btn primary" on:click={() => (showAdd = !showAdd)}>+ Track folder</button>
  </div>
</div>

<OfferedGames />

{#if showAdd}
  <AddGameCard on:close={() => (showAdd = false)} />
{/if}

<ScanDialog bind:this={scanner} bind:scanning />

<div class="stats">
  <div class="card stat">
    <div class="stat-num">{$gameList.length}</div>
    <div class="stat-label">games tracked</div>
  </div>
  <div class="card stat">
    <div class="stat-num">{onlinePeers.length}</div>
    <div class="stat-label">peers online</div>
  </div>
  <div class="card stat">
    <div class="stat-num">{Object.values($syncActivity).filter((s) => s.state === 'running').length}</div>
    <div class="stat-label">active syncs</div>
  </div>
</div>

{#if $gameList.length === 0}
  <div class="welcome">
    <div class="welcome-icon">🎮</div>
    <h3>Welcome to OpenSave</h3>
    <p>Keep your game saves in sync across every device — no accounts, no cloud lock-in. Start by finding your saves:</p>
    <div class="welcome-actions">
      <button class="btn primary" on:click={() => scanner.start()} disabled={scanning}>
        {scanning ? 'Scanning…' : '🔍 Auto-scan for saves'}
      </button>
      <button class="btn" on:click={() => (showAdd = true)}>+ Track a folder manually</button>
    </div>
    <p class="welcome-hint">Then open <strong>Devices</strong> to pair another PC or Steam Deck, or <strong>Cloud Backup</strong> to mirror snapshots online.</p>
  </div>
{:else}
  <LibraryGrid />
{/if}

<style>
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 20px;
    gap: 12px;
    flex-wrap: wrap;
  }
  .head-actions {
    display: flex;
    gap: 8px;
  }

  .stats {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin-bottom: 26px;
  }
  .stat {
    text-align: center;
    padding: 18px;
  }
  .stat-num {
    font-size: 1.8rem;
    font-weight: 700;
  }
  .stat-label {
    color: var(--text-faint);
    font-size: 0.82rem;
  }

  .welcome {
    text-align: center;
    max-width: 520px;
    margin: 40px auto;
    padding: 40px 28px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }
  .welcome-icon {
    font-size: 3rem;
    margin-bottom: 10px;
  }
  .welcome h3 {
    font-size: 1.3rem;
    font-weight: 700;
    margin-bottom: 10px;
  }
  .welcome p {
    color: var(--text-dim);
    font-size: 0.92rem;
    line-height: 1.55;
  }
  .welcome-actions {
    display: flex;
    gap: 10px;
    justify-content: center;
    margin: 22px 0 6px;
    flex-wrap: wrap;
  }
  .welcome-hint {
    font-size: 0.8rem;
    color: var(--text-faint);
    margin-top: 16px;
  }
</style>
