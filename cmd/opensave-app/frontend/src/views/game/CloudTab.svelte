<script>
  import { t as translateUI } from '../../lib/i18n.js';
  // This game's snapshots in the cloud backup: restore one, or send up the
  // ones that are only here.
  import { navigate, toast, askConfirm, settings } from '../../lib/stores.js';
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
    if (!(await askConfirm($translateUI('cloudTab.downloadAndRestoreCloudSnapshotOverYour', { p0: snap.snapshotId }), { title: $translateUI('cloudBrowser.restoreFromCloud'), confirmText: $translateUI('cloudTab.downloadRestore') }))) return;
    return run($translateUI('cloudBrowser.restoredFromCloud'), async () => {
      await api.post(`/api/cloud/restore/${game.id}`, { fileName: snap.name });
    });
  };

  const upload = () =>
    run($translateUI('cloudTab.uploadedToCloud'), async () => {
      const res = await api.post(`/api/cloud/sync-local/${game.id}`);
      toast($translateUI('cloudBrowser.uploadedSkipped', { p0: res.uploaded, p1: res.skipped }), 'success');
      await load();
    });
</script>

<div class="card">
  <div class="head">
    <div>
      <h3 class="with-icon"><Cloud size={18} /> {$translateUI('cloudTab.cloudSnapshotsFor')} {game.name}</h3>
      <p class="sub">{$translateUI('cloudTab.snapshotsBackedUpToYourConfiguredCloud')}</p>
    </div>
    <div class="actions">
      <button class="btn small" disabled={$busy} on:click={load}><RefreshCw size={13} />{$translateUI('cloudTab.refresh')}</button>
      <button class="btn small primary" disabled={$busy} on:click={upload}><Upload size={13} />{$translateUI('cloudBrowser.uploadLocalSnapshots')}</button>
    </div>
  </div>

  {#if loading}
    <div class="quiet-block"><Spinner size={13} /> {$translateUI('cloudTab.loadingCloudSnapshots')}</div>
  {:else if !snaps || snaps.length === 0}
    <div class="quiet-block">
      <p>{$translateUI('cloudBrowser.noCloudSnapshotsForThisGameYet')}</p>
      {#if $settings?.cloudSync?.ready}
        <p class="hint">{$translateUI('cloudTab.newSnapshotsAreCopiedUpAsThey')}</p>
      {:else}
        <p class="hint">
          {$translateUI('cloudTab.setUpAProviderIn')} <button class="linklike" on:click={() => navigate('cloud')}>{$translateUI('home.cloudBackup')}</button>{$translateUI('cloudTab.thenUseUploadLocalSnapshots')}
        </p>
      {/if}
    </div>
  {:else}
    <table>
      <thead>
        <tr><th>{$translateUI('cloudTab.branch')}</th><th>{$translateUI('cloudTab.date')}</th><th>{$translateUI('cloudTab.size')}</th><th></th></tr>
      </thead>
      <tbody>
        {#each snaps as snap (snap.name)}
          <tr>
            <td><span class="badge offline">{snap.branch}</span></td>
            <td class="mono">{new Date(snap.createdTime).toLocaleString()}</td>
            <td class="mono">{fmtSize(snap.sizeBytes)}</td>
            <td class="right">
              <button class="btn small primary" disabled={$busy} on:click={() => restore(snap)}>{$translateUI('cloudBrowser.restore')}</button>
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
    color: var(--accent-text);
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
