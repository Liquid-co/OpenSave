<script>
  // This game's snapshots in the cloud backup: restore one, or send up the
  // ones that are only here.
  import { navigate, toast, askConfirm } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import { fmtSize } from '../../lib/format.js';
  import Spinner from '../../components/ui/Spinner.svelte';
  import Cloud from 'lucide-svelte/icons/cloud';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import Upload from 'lucide-svelte/icons/upload';

  export let game;
  export let runner;
  const { busy, run } = runner;

  let snaps = null;
  let loading = false;

  async function load() {
    loading = true;
    snaps = null;
    try {
      snaps = await api.get(`/api/cloud/snapshots/${game.id}`);
    } catch (e) {
      toast(e.message, 'error');
      snaps = [];
    } finally {
      loading = false;
    }
  }
  load();

  const restore = async (snap) => {
    if (!(await askConfirm(`Download and restore cloud snapshot ${snap.snapshotId} over your current save?`, { title: 'Restore from cloud?', confirmText: 'Download & restore' }))) return;
    return run('Restored from cloud', async () => {
      await api.post(`/api/cloud/restore/${game.id}`, { fileName: snap.name });
    });
  };

  const upload = () =>
    run('Uploaded to cloud', async () => {
      const res = await api.post(`/api/cloud/sync-local/${game.id}`);
      toast(`Uploaded ${res.uploaded}, skipped ${res.skipped}`, 'success');
      await load();
    });
</script>

<div class="card">
  <div class="head">
    <div>
      <h3 class="with-icon"><Cloud size={18} /> Cloud snapshots for {game.name}</h3>
      <p class="sub">Snapshots backed up to your configured cloud provider.</p>
    </div>
    <div class="actions">
      <button class="btn small" disabled={$busy} on:click={load}><RefreshCw size={13} />Refresh</button>
      <button class="btn small primary" disabled={$busy} on:click={upload}><Upload size={13} />Upload local snapshots</button>
    </div>
  </div>

  {#if loading}
    <div class="quiet-block"><Spinner size={13} /> Loading cloud snapshots…</div>
  {:else if !snaps || snaps.length === 0}
    <div class="quiet-block">
      <p>No cloud snapshots for this game yet.</p>
      <p class="hint">
        Enable a provider in <button class="linklike" on:click={() => navigate('cloud')}>Cloud Backup</button>,
        then use “Upload local snapshots”.
      </p>
    </div>
  {:else}
    <table>
      <thead>
        <tr><th>Branch</th><th>Date</th><th>Size</th><th></th></tr>
      </thead>
      <tbody>
        {#each snaps as snap (snap.name)}
          <tr>
            <td><span class="badge offline">{snap.branch}</span></td>
            <td class="mono">{new Date(snap.createdTime).toLocaleString()}</td>
            <td class="mono">{fmtSize(snap.sizeBytes)}</td>
            <td class="right">
              <button class="btn small primary" disabled={$busy} on:click={() => restore(snap)}>Restore</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<style>
  .head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
    margin-bottom: 14px;
    flex-wrap: wrap;
  }
  .sub {
    font-size: 0.82rem;
    color: var(--text-faint);
    margin-top: 3px;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
  .quiet-block {
    padding: 30px 10px;
    text-align: center;
    color: var(--text-faint);
  }
  .hint {
    font-size: 0.82rem;
    margin-top: 6px;
  }
  .linklike {
    background: none;
    border: none;
    color: var(--accent);
    cursor: pointer;
    padding: 0;
    font: inherit;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem;
  }
  th {
    text-align: left;
    color: var(--text-faint);
    font-weight: 600;
    font-size: 0.75rem;
    padding: 6px 10px;
    border-bottom: 1px solid var(--border);
  }
  td {
    padding: 9px 10px;
    border-bottom: 1px solid var(--border);
  }
  tr:last-child td {
    border-bottom: none;
  }
  .mono {
    font-family: 'Cascadia Code', 'Consolas', monospace;
    font-size: 0.78rem;
    color: var(--text-dim);
  }
  .right {
    text-align: right;
  }
</style>
