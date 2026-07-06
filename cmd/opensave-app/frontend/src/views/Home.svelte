<script>
  import { gameList, peers, navigate, toast, syncActivity } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';

  export let params = {};

  let showAdd = params.add ?? false;
  $: if (params.add) showAdd = true;

  // ── Track-a-game form ────────────────────────────────────────────
  let newName = '';
  let newPath = '';
  let adding = false;

  async function pickFolder() {
    const dir = await native.selectDirectory('Select the save folder to track');
    if (dir) newPath = dir;
  }

  async function addGame() {
    if (!newName || !newPath || adding) return;
    adding = true;
    try {
      const game = await api.post('/api/games', { name: newName, savePath: newPath });
      toast(`Now tracking "${newName}"`, 'success');
      newName = '';
      newPath = '';
      showAdd = false;
      navigate('game', { gameId: game.id });
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      adding = false;
    }
  }

  // ── Auto-scan ────────────────────────────────────────────────────
  let scanning = false;
  let scanResults = null;

  async function scan() {
    scanning = true;
    scanResults = null;
    try {
      scanResults = await api.get('/api/presets/scan');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      scanning = false;
    }
  }

  async function trackDetected(item) {
    try {
      const game = await api.post('/api/games', {
        name: item.name,
        savePath: item.savePath,
        appId: item.appId ?? ''
      });
      toast(`Now tracking "${item.name}"`, 'success');
      scanResults = scanResults.filter((r) => r.id !== item.id);
      navigate('game', { gameId: game.id });
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function syncAll() {
    for (const g of $gameList) {
      api.post(`/api/games/${g.id}/sync`).catch(() => {});
    }
    toast('Sync triggered for all games');
  }

  $: onlinePeers = Object.values($peers).filter((p) => p.status === 'online');
  const typeLabels = { emulator: 'Emulator', repack: 'Repack', game: 'Game' };
</script>

<div class="head">
  <h2 class="page-title">Home</h2>
  <div class="head-actions">
    <button class="btn" on:click={scan} disabled={scanning}>
      {scanning ? 'Scanning…' : '🔍 Auto-scan'}
    </button>
    <button class="btn" on:click={syncAll} disabled={$gameList.length === 0}>⟳ Sync all</button>
    <button class="btn primary" on:click={() => (showAdd = !showAdd)}>+ Track folder</button>
  </div>
</div>

{#if showAdd}
  <div class="card add-card">
    <h3>Track a save folder</h3>
    <div class="row">
      <div class="field grow">
        <label for="g-name">Game name</label>
        <input id="g-name" placeholder="e.g. Elden Ring" bind:value={newName} />
      </div>
      <div class="field grow2">
        <label for="g-path">Save folder or file</label>
        <div class="path-row">
          <input id="g-path" placeholder="C:\Users\you\AppData\…" bind:value={newPath} />
          <button class="btn" on:click={pickFolder}>Browse</button>
        </div>
      </div>
    </div>
    <div class="add-actions">
      <button class="btn" on:click={() => (showAdd = false)}>Cancel</button>
      <button class="btn primary" disabled={!newName || !newPath || adding} on:click={addGame}>
        {adding ? 'Adding…' : 'Start tracking'}
      </button>
    </div>
  </div>
{/if}

{#if scanResults}
  <div class="card scan-card">
    <div class="scan-head">
      <h3>Detected saves ({scanResults.length})</h3>
      <button class="btn small" on:click={() => (scanResults = null)}>Dismiss</button>
    </div>
    {#if scanResults.length === 0}
      <p class="none">Nothing detected — you can still track any folder manually.</p>
    {:else}
      <div class="scan-list">
        {#each scanResults as item (item.id)}
          <div class="scan-item">
            {#if item.appId}
              <img
                class="scan-thumb"
                src={`https://cdn.cloudflare.steamstatic.com/steam/apps/${item.appId}/capsule_231x87.jpg`}
                alt=""
                loading="lazy"
                on:error={(e) => e.currentTarget.remove()}
              />
            {/if}
            <div class="scan-info">
              <div class="scan-name">{item.name}</div>
              <div class="scan-path" title={item.savePath}>{item.savePath}</div>
            </div>
            <span class="badge offline">{typeLabels[item.type] ?? item.type}</span>
            <button class="btn small primary" on:click={() => trackDetected(item)}>Track</button>
          </div>
        {/each}
      </div>
    {/if}
  </div>
{/if}

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
  <div class="empty">
    <h3>Your library is empty</h3>
    <p>Run an auto-scan to find emulator and game saves, or track any folder manually.</p>
  </div>
{:else}
  <h3 class="section">Library</h3>
  <div class="grid">
    {#each $gameList as game (game.id)}
      <button class="card game-card" on:click={() => navigate('game', { gameId: game.id })}>
        <div class="gc-cover">
          {#if game.coverUrl}
            <img src={game.coverUrl} alt="" loading="lazy" on:error={(e) => e.currentTarget.remove()} />
          {/if}
          <div class="gc-cover-fallback"><span>{game.name}</span></div>
        </div>
        <div class="gc-body">
          <div class="gc-name">{game.name}</div>
          <div class="gc-meta">
            branch <strong>{game.activeBranch}</strong>
            · {Object.values(game.branches ?? {}).reduce((n, b) => n + (b.snapshots?.length ?? 0), 0)} snapshots
          </div>
          <div class="gc-path" title={game.savePath}>{game.savePath}</div>
          {#if $syncActivity[game.id]?.state === 'running'}
            <div class="gc-sync">syncing… {$syncActivity[game.id].percentage ?? 0}%</div>
          {/if}
        </div>
      </button>
    {/each}
  </div>
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
  .add-card,
  .scan-card {
    margin-bottom: 20px;
  }
  .add-card h3,
  .scan-card h3 {
    margin-bottom: 14px;
  }
  .row {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
  }
  .grow {
    flex: 1;
    min-width: 180px;
  }
  .grow2 {
    flex: 2;
    min-width: 260px;
  }
  .path-row {
    display: flex;
    gap: 8px;
  }
  .path-row input {
    flex: 1;
  }
  .add-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
  .scan-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 10px;
  }
  .scan-list {
    max-height: min(60vh, 620px);
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding-right: 4px;
  }
  .scan-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 9px 12px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .scan-thumb {
    width: 62px;
    height: 24px;
    object-fit: cover;
    border-radius: 5px;
    flex-shrink: 0;
  }
  .scan-info {
    flex: 1;
    min-width: 0;
  }
  .scan-name {
    font-weight: 600;
    font-size: 0.9rem;
  }
  .scan-path {
    font-size: 0.75rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .none {
    color: var(--text-faint);
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

  .section {
    margin-bottom: 12px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 12px;
  }
  .game-card {
    text-align: left;
    cursor: pointer;
    color: var(--text);
    transition: border-color 0.12s, transform 0.12s;
    padding: 0;
    overflow: hidden;
  }
  .game-card:hover {
    border-color: var(--border-strong);
    transform: translateY(-1px);
  }
  .gc-cover {
    position: relative;
    aspect-ratio: 460 / 175;
    background: var(--bg);
    border-bottom: 1px solid var(--border);
  }
  .gc-cover img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    z-index: 1;
  }
  .gc-cover-fallback {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 12px;
    background: linear-gradient(135deg, rgba(138, 99, 244, 0.16), rgba(138, 99, 244, 0.04));
  }
  .gc-cover-fallback span {
    font-weight: 700;
    font-size: 1.05rem;
    color: var(--text-dim);
    text-align: center;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .gc-body {
    padding: 14px 16px;
  }
  .gc-name {
    font-weight: 600;
    margin-bottom: 6px;
  }
  .gc-meta {
    font-size: 0.8rem;
    color: var(--text-dim);
    margin-bottom: 6px;
  }
  .gc-path {
    font-size: 0.73rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .gc-sync {
    margin-top: 8px;
    font-size: 0.78rem;
    color: var(--accent);
    font-weight: 600;
  }
</style>
