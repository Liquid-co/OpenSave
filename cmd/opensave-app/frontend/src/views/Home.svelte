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

  // ── Auto-scan overlay ────────────────────────────────────────────
  let scanOpen = false;
  let scanFilter = '';
  let scanType = 'all';
  let selected = new Set();
  let selectedCount = 0; // reactive mirror of selected.size

  async function scan() {
    scanning = true;
    scanOpen = true;
    scanResults = null;
    selected = new Set();
    selectedCount = 0;
    try {
      scanResults = await api.get('/api/presets/scan');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      scanning = false;
    }
  }

  function closeScan() {
    scanOpen = false;
    scanResults = null;
  }

  $: trackedPaths = new Set($gameList.map((g) => (g.savePath ?? '').toLowerCase()));
  $: filteredResults = (scanResults ?? []).filter((r) => {
    if (trackedPaths.has((r.savePath ?? '').toLowerCase())) return false;
    if (scanType !== 'all' && r.type !== scanType) return false;
    if (scanFilter && !`${r.name} ${r.savePath}`.toLowerCase().includes(scanFilter.toLowerCase())) return false;
    return true;
  });
  $: scanCounts = {
    all: (scanResults ?? []).length,
    emulator: (scanResults ?? []).filter((r) => r.type === 'emulator').length,
    repack: (scanResults ?? []).filter((r) => r.type === 'repack').length,
    game: (scanResults ?? []).filter((r) => r.type === 'game').length
  };

  function toggleSelect(id) {
    if (selected.has(id)) selected.delete(id);
    else selected.add(id);
    selected = selected; // trigger reactivity
    selectedCount = selected.size;
  }
  function selectAllVisible() {
    for (const r of filteredResults) selected.add(r.id);
    selected = selected;
    selectedCount = selected.size;
  }
  function clearSelection() {
    selected = new Set();
    selectedCount = 0;
  }

  async function trackDetected(item, keepOpen = false) {
    try {
      await api.post('/api/games', { name: item.name, savePath: item.savePath, appId: item.appId ?? '' });
      if (scanResults) scanResults = scanResults.filter((r) => r.id !== item.id);
      selected.delete(item.id);
      selectedCount = selected.size;
      if (!keepOpen) toast(`Now tracking "${item.name}"`, 'success');
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function trackSelected() {
    const items = (scanResults ?? []).filter((r) => selected.has(r.id));
    if (items.length === 0) return;
    let ok = 0;
    for (const item of items) {
      try {
        await api.post('/api/games', { name: item.name, savePath: item.savePath, appId: item.appId ?? '' });
        ok++;
      } catch {}
    }
    scanResults = (scanResults ?? []).filter((r) => !selected.has(r.id));
    clearSelection();
    toast(`Tracked ${ok} game${ok === 1 ? '' : 's'}`, 'success');
    if (filteredResults.length === 0) closeScan();
  }

  async function syncAll() {
    for (const g of $gameList) {
      api.post(`/api/games/${g.id}/sync`).catch(() => {});
    }
    toast('Sync triggered for all games');
  }

  $: onlinePeers = Object.values($peers).filter((p) => p.status === 'online');
  const typeLabels = { emulator: 'Emulator', repack: 'Repack', game: 'Game' };
  const capsuleUrl = (appId) =>
    `https://cdn.cloudflare.steamstatic.com/steam/apps/${appId}/capsule_231x87.jpg`;
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

{#if scanOpen}
  <div class="scan-overlay" on:click|self={closeScan} role="presentation">
    <div class="scan-modal">
      <div class="scan-modal-head">
        <div>
          <h2>🔍 Auto-scan results</h2>
          <p class="scan-modal-sub">
            {#if scanning}Scanning your system…{:else}Found {scanCounts.all} save location{scanCounts.all === 1 ? '' : 's'} — {filteredResults.length} available to track{/if}
          </p>
        </div>
        <button class="btn icon" on:click={closeScan} title="Close">✕</button>
      </div>

      {#if scanning}
        <div class="scan-loading"><span class="cspin"></span> Scanning Steam, emulators, and configured folders…</div>
      {:else}
        <div class="scan-toolbar">
          <input class="scan-search" placeholder="Filter by name or path…" bind:value={scanFilter} />
          <div class="scan-type-tabs">
            {#each [['all', 'All'], ['game', 'Games'], ['emulator', 'Emulators'], ['repack', 'Repacks']] as [id, label]}
              <button class:active={scanType === id} on:click={() => (scanType = id)}>
                {label} <span class="count">{scanCounts[id]}</span>
              </button>
            {/each}
          </div>
        </div>

        <div class="scan-modal-list">
          {#each filteredResults as item (item.id)}
            <label class="scan-row" class:sel={selected.has(item.id)}>
              <input type="checkbox" checked={selected.has(item.id)} on:change={() => toggleSelect(item.id)} />
              <div class="scan-thumb-box">
                {#if item.appId}
                  <img src={capsuleUrl(item.appId)} alt="" loading="lazy" on:error={(e) => (e.currentTarget.style.display = 'none')} />
                {:else}
                  <span class="scan-thumb-fallback">{item.type === 'emulator' ? '🕹️' : item.type === 'repack' ? '📦' : '🎮'}</span>
                {/if}
              </div>
              <div class="scan-info">
                <div class="scan-name">{item.name}</div>
                <div class="scan-path" title={item.savePath}>{item.savePath}</div>
              </div>
              <span class="badge offline">{typeLabels[item.type] ?? item.type}</span>
              <button class="btn small primary" on:click|preventDefault={() => trackDetected(item)}>Track</button>
            </label>
          {:else}
            <div class="scan-empty">
              {scanCounts.all === 0 ? 'Nothing detected. You can still track any folder manually.' : 'No matches for this filter.'}
            </div>
          {/each}
        </div>

        <div class="scan-modal-foot">
          <div class="scan-select-actions">
            <button class="btn small" on:click={selectAllVisible} disabled={filteredResults.length === 0}>Select all ({filteredResults.length})</button>
            {#if selectedCount > 0}
              <button class="btn small" on:click={clearSelection}>Clear</button>
            {/if}
          </div>
          <button class="btn primary" disabled={selectedCount === 0} on:click={trackSelected}>
            Track selected ({selectedCount})
          </button>
        </div>
      {/if}
    </div>
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
  .add-card {
    margin-bottom: 20px;
  }
  .add-card h3 {
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
  /* Auto-scan overlay */
  .scan-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.62);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 80;
    padding: 32px;
  }
  .scan-modal {
    width: min(920px, 100%);
    height: min(82vh, 860px);
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    display: flex;
    flex-direction: column;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
  }
  .scan-modal-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    padding: 20px 22px 14px;
    border-bottom: 1px solid var(--border);
  }
  .scan-modal-head h2 {
    font-size: 1.2rem;
  }
  .scan-modal-sub {
    font-size: 0.84rem;
    color: var(--text-faint);
    margin-top: 3px;
  }
  .scan-loading {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    color: var(--text-dim);
  }
  .cspin {
    width: 14px;
    height: 14px;
    border: 2px solid var(--accent-soft);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  .scan-toolbar {
    display: flex;
    gap: 12px;
    padding: 14px 22px;
    align-items: center;
    flex-wrap: wrap;
  }
  .scan-search {
    flex: 1;
    min-width: 200px;
    padding: 9px 13px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
    outline: none;
  }
  .scan-type-tabs {
    display: flex;
    gap: 6px;
  }
  .scan-type-tabs button {
    padding: 7px 13px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: transparent;
    color: var(--text-dim);
    font-size: 0.85rem;
    cursor: pointer;
  }
  .scan-type-tabs button:hover {
    background: var(--bg-hover);
  }
  .scan-type-tabs button.active {
    background: var(--bg-active);
    color: var(--text);
    border-color: var(--border-strong);
  }
  .scan-type-tabs .count {
    color: var(--text-faint);
    font-size: 0.75rem;
    margin-left: 2px;
  }
  .scan-modal-list {
    flex: 1;
    overflow-y: auto;
    padding: 0 22px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .scan-row {
    display: flex;
    align-items: center;
    gap: 13px;
    padding: 9px 12px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    cursor: pointer;
  }
  .scan-row:hover {
    border-color: var(--border-strong);
  }
  .scan-row.sel {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .scan-row input[type='checkbox'] {
    accent-color: var(--accent);
    width: 16px;
    height: 16px;
    flex-shrink: 0;
  }
  .scan-thumb-box {
    width: 66px;
    height: 30px;
    flex-shrink: 0;
    border-radius: 5px;
    overflow: hidden;
    background: var(--bg-active);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .scan-thumb-box img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .scan-thumb-fallback {
    font-size: 1rem;
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
  .scan-empty {
    text-align: center;
    color: var(--text-faint);
    padding: 50px 20px;
  }
  .scan-modal-foot {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 14px 22px;
    border-top: 1px solid var(--border);
  }
  .scan-select-actions {
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
