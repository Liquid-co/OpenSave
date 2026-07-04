<script>
  import { games, navigate, toast, syncActivity } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';

  export let params = {};

  $: game = $games[params.gameId];
  $: activity = $syncActivity[params.gameId];

  let tab = 'snapshots';
  let newBranch = '';
  let snapshotComment = '';
  let busy = false;
  let browsing = null; // {snapshotId, files}

  $: branches = game ? Object.values(game.branches ?? {}) : [];
  $: allSnapshots = branches
    .flatMap((b) => (b.snapshots ?? []).map((s) => ({ ...s, branch: b.name })))
    .sort((a, b) => (a.timestamp < b.timestamp ? 1 : -1));

  const fmtTime = (t) => (t ? new Date(t).toLocaleString() : '—');
  const fmtSize = (n) => (n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : (n / 1024).toFixed(1) + ' KB');

  async function run(label, fn) {
    if (busy) return;
    busy = true;
    try {
      await fn();
      if (label) toast(label, 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  const syncNow = () => run('Sync triggered', () => api.post(`/api/games/${game.id}/sync`));
  const takeSnapshot = () =>
    run('Snapshot created', async () => {
      await api.post(`/api/games/${game.id}/snapshot`, { comment: snapshotComment });
      snapshotComment = '';
    });
  const rollback = (snap) =>
    run(`Restored ${snap.id}`, () => api.post(`/api/games/${game.id}/rollback`, { snapshotId: snap.id }));
  const createBranch = () =>
    run('Branch created', async () => {
      await api.post(`/api/games/${game.id}/branch`, { name: newBranch });
      newBranch = '';
    });
  const switchBranch = (name) =>
    run(`Switched to "${name}"`, () => api.post(`/api/games/${game.id}/branch/switch`, { name }));

  async function browseSnapshot(snap) {
    try {
      const files = await api.get(`/api/games/${game.id}/snapshot/${snap.id}/files`);
      browsing = { snapshotId: snap.id, files };
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  const restoreFile = (relPath) =>
    run(`Restored ${relPath}`, () =>
      api.post(`/api/games/${game.id}/snapshot/${browsing.snapshotId}/restore-file`, { relPath })
    );

  async function untrack() {
    if (!confirm(`Stop tracking "${game.name}"? Snapshot files stay on disk.`)) return;
    await run('Stopped tracking', () => api.del(`/api/games/${game.id}`));
    navigate('home');
  }

  async function launch() {
    if (game.appId) {
      native.openExternal(`steam://run/${game.appId}`);
    } else if (game.exePath) {
      toast('Launching…');
      api.post(`/api/games/${game.id}/launch`).catch((e) => toast(e.message, 'error'));
    }
  }

  let editPath = false;
  let pathDraft = '';
  async function savePath() {
    await run('Save path updated', () => api.patch(`/api/games/${game.id}`, { savePath: pathDraft }));
    editPath = false;
  }
</script>

{#if !game}
  <div class="empty"><h3>Game not found</h3></div>
{:else}
  <div class="head">
    <button class="btn icon back" on:click={() => navigate('home')} title="Back">←</button>
    <div class="title-block">
      <h2 class="page-title">{game.name}</h2>
      <div class="sub">
        branch <strong>{game.activeBranch}</strong>
        {#if activity?.state === 'running'}
          · <span class="syncing">syncing {activity.percentage ?? 0}%</span>
        {/if}
      </div>
    </div>
    <div class="head-actions">
      {#if game.appId || game.exePath}
        <button class="btn" on:click={launch}>▶ Launch</button>
      {/if}
      <button class="btn primary" disabled={busy} on:click={syncNow}>⟳ Sync now</button>
    </div>
  </div>

  <div class="path-line">
    {#if editPath}
      <input class="path-input" bind:value={pathDraft} />
      <button class="btn small" on:click={async () => (pathDraft = (await native.selectDirectory('Select save folder')) || pathDraft)}>Browse</button>
      <button class="btn small primary" on:click={savePath}>Save</button>
      <button class="btn small" on:click={() => (editPath = false)}>Cancel</button>
    {:else}
      <span class="path" title={game.savePath}>{game.savePath}</span>
      <button class="btn small" on:click={() => { pathDraft = game.savePath; editPath = true; }}>Edit</button>
    {/if}
  </div>

  <div class="pill-tabs tabs">
    <button class:active={tab === 'snapshots'} on:click={() => (tab = 'snapshots')}>Snapshots</button>
    <button class:active={tab === 'branches'} on:click={() => (tab = 'branches')}>Branches</button>
    <button class:active={tab === 'danger'} on:click={() => (tab = 'danger')}>Manage</button>
  </div>

  {#if tab === 'snapshots'}
    <div class="card snap-new">
      <input placeholder="Snapshot comment (optional)" bind:value={snapshotComment} />
      <button class="btn primary" disabled={busy} on:click={takeSnapshot}>📸 Snapshot now</button>
    </div>

    {#if browsing}
      <div class="card browse">
        <div class="browse-head">
          <h3>Files in {browsing.snapshotId}</h3>
          <button class="btn small" on:click={() => (browsing = null)}>Close</button>
        </div>
        {#each browsing.files.filter((f) => !f.isDir) as f}
          <div class="file-row">
            <span class="file-path">{f.path}</span>
            <span class="file-size">{fmtSize(f.size)}</span>
            <button class="btn small" disabled={busy} on:click={() => restoreFile(f.path)}>Restore file</button>
          </div>
        {/each}
      </div>
    {/if}

    {#if allSnapshots.length === 0}
      <div class="empty"><h3>No snapshots yet</h3><p>Snapshots are created automatically when your save changes.</p></div>
    {:else}
      <div class="snap-list">
        {#each allSnapshots as snap (snap.id)}
          <div class="card snap">
            <div class="snap-info">
              <div class="snap-top">
                <span class="snap-id">{snap.id}</span>
                <span class="badge offline">{snap.branch}</span>
                {#if snap.isSystemAuto}<span class="badge offline">auto</span>{/if}
              </div>
              <div class="snap-comment">{snap.comment}</div>
              <div class="snap-meta">{fmtTime(snap.timestamp)} · {fmtSize(snap.sizeBytes)}</div>
            </div>
            <div class="snap-actions">
              <button class="btn small" on:click={() => browseSnapshot(snap)}>Browse files</button>
              <button class="btn small primary" disabled={busy} on:click={() => rollback(snap)}>Restore</button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {:else if tab === 'branches'}
    <div class="card snap-new">
      <input placeholder="New branch name (e.g. ng-plus)" bind:value={newBranch} />
      <button class="btn primary" disabled={!newBranch || busy} on:click={createBranch}>+ Create branch</button>
    </div>
    <div class="snap-list">
      {#each branches as branch (branch.name)}
        <div class="card snap">
          <div class="snap-info">
            <div class="snap-top">
              <span class="snap-id">{branch.name}</span>
              {#if branch.name === game.activeBranch}<span class="badge online">active</span>{/if}
            </div>
            <div class="snap-meta">{branch.snapshots?.length ?? 0} snapshot(s)</div>
          </div>
          {#if branch.name !== game.activeBranch}
            <button class="btn small primary" disabled={busy} on:click={() => switchBranch(branch.name)}>
              Switch to
            </button>
          {/if}
        </div>
      {/each}
    </div>
    <p class="branch-hint">
      Switching branches snapshots your current save first, then restores the other branch's latest state.
    </p>
  {:else}
    <div class="card">
      <h3>Stop tracking</h3>
      <p class="danger-desc">
        Removes "{game.name}" from OpenSave. Your save files and existing snapshot archives on disk are
        kept.
      </p>
      <button class="btn danger" disabled={busy} on:click={untrack}>Stop tracking this game</button>
    </div>
  {/if}
{/if}

<style>
  .head {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-bottom: 6px;
  }
  .back {
    font-size: 1rem;
  }
  .title-block {
    flex: 1;
    min-width: 0;
  }
  .sub {
    color: var(--text-dim);
    font-size: 0.85rem;
    margin-top: 2px;
  }
  .syncing {
    color: var(--accent);
    font-weight: 600;
  }
  .head-actions {
    display: flex;
    gap: 8px;
  }
  .path-line {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 0 18px 50px;
  }
  .path {
    font-size: 0.8rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .path-input {
    flex: 1;
    padding: 6px 10px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.82rem;
    outline: none;
  }
  .tabs {
    margin-bottom: 18px;
  }
  .snap-new {
    display: flex;
    gap: 10px;
    padding: 14px;
    margin-bottom: 14px;
  }
  .snap-new input {
    flex: 1;
    padding: 8px 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
    outline: none;
  }
  .snap-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .snap {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 16px;
  }
  .snap-info {
    flex: 1;
    min-width: 0;
  }
  .snap-top {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 4px;
  }
  .snap-id {
    font-weight: 600;
    font-size: 0.9rem;
  }
  .snap-comment {
    font-size: 0.83rem;
    color: var(--text-dim);
    margin-bottom: 3px;
  }
  .snap-meta {
    font-size: 0.75rem;
    color: var(--text-faint);
  }
  .snap-actions {
    display: flex;
    gap: 6px;
  }
  .browse {
    margin-bottom: 14px;
  }
  .browse-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 10px;
  }
  .file-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 7px 4px;
    border-bottom: 1px solid var(--border);
    font-size: 0.85rem;
  }
  .file-row:last-child {
    border-bottom: none;
  }
  .file-path {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .file-size {
    color: var(--text-faint);
    font-size: 0.78rem;
  }
  .branch-hint {
    margin-top: 12px;
    font-size: 0.8rem;
    color: var(--text-faint);
  }
  .danger-desc {
    color: var(--text-dim);
    font-size: 0.88rem;
    margin: 8px 0 14px;
  }
</style>
