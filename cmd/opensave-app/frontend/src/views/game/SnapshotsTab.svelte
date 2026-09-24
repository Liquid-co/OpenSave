<script>
  // Every snapshot of this game, newest first, on every branch: take one,
  // restore one, look inside one and put back a single file.
  import { askConfirm, toast } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import { fmtSize, fmtTime } from '../../lib/format.js';

  export let game;
  export let runner;
  const { busy, run } = runner;

  let comment = '';
  let browsing = null; // {snapshotId, files}

  $: allSnapshots = Object.values(game.branches ?? {})
    .flatMap((b) => (b.snapshots ?? []).map((s) => ({ ...s, branch: b.name })))
    .sort((a, b) => (a.timestamp < b.timestamp ? 1 : -1));

  const takeSnapshot = () =>
    run('Snapshot created', async () => {
      await api.post(`/api/games/${game.id}/snapshot`, { comment });
      comment = '';
    });

  const rollback = async (snap) => {
    if (!(await askConfirm(`Restore snapshot ${snap.id} over your current save? Your current state is snapshotted first, so this is reversible.`, { title: 'Restore snapshot?', confirmText: 'Restore' }))) return;
    return run(`Restored ${snap.id}`, () => api.post(`/api/games/${game.id}/rollback`, { snapshotId: snap.id }));
  };

  async function deleteSnapshot(snap) {
    if (!(await askConfirm(`Delete snapshot ${snap.id}? This can't be undone; your current save isn't affected.`, { title: 'Delete snapshot?', confirmText: 'Delete', danger: true }))) return;
    run('Snapshot deleted', () => api.del(`/api/games/${game.id}/snapshot/${snap.id}`));
  }

  async function browseSnapshot(snap) {
    try {
      const files = await api.get(`/api/games/${game.id}/snapshot/${snap.id}/files`);
      browsing = { snapshotId: snap.id, files };
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  const restoreFile = async (relPath) => {
    if (!(await askConfirm(`Restore "${relPath}" from ${browsing.snapshotId} over the current file?`, { title: 'Restore file?', confirmText: 'Restore' }))) return;
    return run(`Restored ${relPath}`, () =>
      api.post(`/api/games/${game.id}/snapshot/${browsing.snapshotId}/restore-file`, { relPath })
    );
  };
</script>

<div class="card snap-new">
  <input placeholder="Snapshot comment (optional)" bind:value={comment} />
  <button class="btn primary" disabled={$busy} on:click={takeSnapshot}>📸 Snapshot now</button>
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
        <button class="btn small" disabled={$busy} on:click={() => restoreFile(f.path)}>Restore file</button>
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
          <button class="btn small primary" disabled={$busy} on:click={() => rollback(snap)}>Restore</button>
          <button class="btn small danger" disabled={$busy} on:click={() => deleteSnapshot(snap)}>Delete</button>
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
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
  /* The same class in the configuration tab is a different row; in one
     component they overrode each other, which gave these the picker's
     padding and a pointer cursor over rows that aren't clickable. */
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
</style>
