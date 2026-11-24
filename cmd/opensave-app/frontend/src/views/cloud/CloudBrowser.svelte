<script>
  // Every game with snapshots in the cloud, as tiles — and every tracked game
  // with none there yet, so it can be sent up. Click one to restore, delete
  // or upload its snapshots. Fires `close`.
  import { createEventDispatcher, onDestroy } from 'svelte';
  import { gameList, toast, cloudUploadEvent, askConfirm } from '../../lib/stores.js';
  import { api, gameCover } from '../../lib/api.js';
  import { cloudTiles } from '../../lib/cloudproviders.js';
  import { fmtSize } from '../../lib/format.js';
  import { t } from '../../lib/i18n.js';
  import Modal from '../../components/ui/Modal.svelte';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import ModalLoading from '../../components/ui/ModalLoading.svelte';
  import FilterTabs from '../../components/ui/FilterTabs.svelte';
  import SearchInput from '../../components/ui/SearchInput.svelte';
  import CoverTile from '../../components/ui/CoverTile.svelte';
  import ProgressBar from '../../components/ui/ProgressBar.svelte';
  import Cloud from 'lucide-svelte/icons/cloud';
  import HardDrive from 'lucide-svelte/icons/hard-drive';
  import ArrowLeft from 'lucide-svelte/icons/arrow-left';
  import Upload from 'lucide-svelte/icons/upload';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';

  /** The page's busy flag (a writable store). */
  export let busy;
  /** Called when the provider says the sign-in is gone. */
  export let onAuthLost = () => {};

  const dispatch = createEventDispatcher();
  const close = () => dispatch('close');

  let cloudGames = null; // null = not loaded yet
  let browsing = false;
  let filter = '';
  let tab = 'all'; // all | cloud | local
  let detailId = null; // gameId drilled into (null = tile grid)
  // An upload runs apart from `busy`, so Restore and Delete don't grey out
  // while snapshots are being pushed up.
  let uploading = false;
  let uploadProg = null; // live {done, total, current} from the daemon

  const unsubUpload = cloudUploadEvent.subscribe((ev) => {
    if (!ev) return;
    uploadProg = ev.complete ? null : ev;
  });
  onDestroy(unsubUpload);

  // A cloud call that fails on expired credentials means the daemon has
  // just wiped the dead tokens — the page reloads so the "connected" badge
  // flips to "sign in" instead of lying about a working connection.
  function handleCloudError(e) {
    toast(e.message, 'error');
    if (/expired|reconnect|not authenticated|re-auth/i.test(e.message)) onAuthLost();
  }

  async function browse() {
    browsing = true;
    // Keep the current listing visible while refreshing so the detail view
    // doesn't flash back to the loading spinner mid-upload.
    try {
      cloudGames = await api.get('/api/cloud/browse');
    } catch (e) {
      handleCloudError(e);
    } finally {
      browsing = false;
    }
  }
  browse();

  $: tiles = cloudTiles(cloudGames, $gameList, (g) => gameCover(g, true));
  $: tabCounts = {
    all: tiles.length,
    cloud: tiles.filter((t) => t.cloud).length,
    local: tiles.filter((t) => !t.cloud).length
  };
  $: filtered = tiles.filter(
    (t) =>
      (tab === 'all' || (tab === 'cloud' ? !!t.cloud : !t.cloud)) &&
      t.name.toLowerCase().includes(filter.trim().toLowerCase())
  );
  $: detail = detailId ? tiles.find((t) => t.id === detailId) : null;
  $: totals = (cloudGames ?? []).reduce((a, g) => ({ snaps: a.snaps + g.count, size: a.size + g.totalSize }), {
    snaps: 0,
    size: 0
  });

  // Delete only removes the remote copy — local snapshots stay put.
  const deleteCloud = async (g, f) => {
    if (
      !(await askConfirm(
        $t('cloud.browser.deleteConfirm', { id: f.snapshotId, game: g.gameName }),
        { title: $t('cloud.browser.deleteConfirmTitle'), confirmText: $t('common.delete'), danger: true }
      ))
    )
      return;
    busy.set(true);
    try {
      await api.post(`/api/cloud/delete/${g.gameId}`, { fileName: f.name, id: f.id ?? '' });
      g.snapshots = g.snapshots.filter((x) => x.name !== f.name);
      g.count = g.snapshots.length;
      g.totalSize = g.snapshots.reduce((n, x) => n + x.sizeBytes, 0);
      cloudGames = cloudGames.filter((x) => x.count > 0);
      toast($t('cloud.browser.deletedToast'), 'success');
    } catch (e) {
      handleCloudError(e);
    } finally {
      busy.set(false);
    }
  };

  const restoreCloud = async (gameId, file) => {
    if (!(await askConfirm($t('cloud.browser.restoreConfirm', { id: file.snapshotId }), { title: $t('game.cloud.restoreTitle'), confirmText: $t('game.snapshots.restore') }))) return;
    busy.set(true);
    try {
      await api.post(`/api/cloud/restore/${gameId}`, { fileName: file.name });
      toast($t('game.cloud.restored'), 'success');
    } catch (e) {
      handleCloudError(e);
    } finally {
      busy.set(false);
    }
  };

  const uploadLocal = async (gameId) => {
    uploading = true;
    try {
      const res = await api.post(`/api/cloud/sync-local/${gameId}`);
      toast(
        res.uploaded === 0 && res.skipped > 0
          ? $t('cloud.browser.allUpToDate', { skipped: res.skipped })
          : $t('game.cloud.uploadResult', { uploaded: res.uploaded, skipped: res.skipped }),
        'success'
      );
      await browse(); // the detail re-derives from the fresh listing
    } catch (e) {
      handleCloudError(e);
    } finally {
      uploading = false;
      uploadProg = null;
    }
  };

  $: tabOptions = [
    ['all', $t('cloud.browser.tabAll')],
    ['cloud', $t('cloud.browser.tabCloud')],
    ['local', $t('cloud.browser.tabLocal')]
  ];
</script>

<Modal title={$t('cloud.backup.snapshotsSection')} icon={Cloud} onClose={close}>
  <svelte:fragment slot="sub">
    {#if browsing}
      {$t('cloud.browser.reading')}
    {:else if cloudGames}
      {$t('cloud.browser.summaryGames', { n: cloudGames.length })} · {$t('cloud.browser.summarySnaps', { n: totals.snaps })}
      · {$t('cloud.browser.summarySize', { size: fmtSize(totals.size) })}
    {:else}
      {$t('cloud.browser.readFailed')}
    {/if}
  </svelte:fragment>
  <svelte:fragment slot="actions">
    <button class="btn small" disabled={browsing} on:click={browse}>
      <RefreshCw size={14} />{browsing ? $t('cloud.browser.loading') : $t('game.cloud.refresh')}
    </button>
  </svelte:fragment>

  {#if browsing && !cloudGames}
    <ModalLoading>{$t('cloud.browser.loadingList')}</ModalLoading>
  {:else if cloudGames && detail}
    <!-- drill-in: one game's cloud snapshots -->
    <div class="detail-head">
      <button class="btn small" on:click={() => (detailId = null)}><ArrowLeft size={14} />{$t('game.header.back')}</button>
      <div class="detail-title">
        <strong>{detail.name}</strong>
        <span class="quiet">
          {#if detail.cloud}
            {$t('cloud.browser.summarySnaps', { n: detail.cloud.count })} · {$t('cloud.browser.summarySize', { size: fmtSize(detail.cloud.totalSize) })}
          {:else}
            {$t('cloud.browser.nothingYet')}
          {/if}
        </span>
      </div>
      {#if detail.tracked}
        <button class="btn small" disabled={uploading} on:click={() => uploadLocal(detail.id)}>
          <Upload size={14} />{uploading ? $t('cloud.browser.uploading') : $t('game.cloud.uploadBtn')}
        </button>
      {/if}
    </div>
    {#if uploading}
      <ProgressBar done={uploadProg?.done ?? 0} total={uploadProg?.total ?? 0}>
        {#if uploadProg && uploadProg.total > 0}
          {$t('cloud.browser.uploadProgress', { done: Math.min(uploadProg.done + 1, uploadProg.total), total: uploadProg.total })}
          {#if uploadProg.current}&nbsp;— <code>{uploadProg.current}</code>{/if}
        {:else}
          {$t('cloud.browser.checkingUploads')}
        {/if}
      </ProgressBar>
    {/if}
    <div class="list">
      {#if detail.cloud}
        {#each detail.cloud.snapshots as f (f.name)}
          <div class="row">
            <div class="info">
              <div class="name">{f.snapshotId} <span class="badge offline">{f.branch}</span></div>
              <div class="meta">{fmtSize(f.sizeBytes)} · {new Date(f.createdTime).toLocaleString()}</div>
            </div>
            <button
              class="btn small primary"
              disabled={$busy || !detail.tracked}
              title={detail.tracked ? $t('cloud.browser.restoreHint') : $t('cloud.browser.restoreTrackFirst')}
              on:click={() => restoreCloud(detail.id, f)}
            >
              {$t('game.snapshots.restore')}
            </button>
            <button
              class="btn small danger"
              disabled={$busy}
              title={$t('cloud.browser.deleteHint')}
              on:click={() => deleteCloud(detail.cloud, f)}
            >
              {$t('common.delete')}
            </button>
          </div>
        {/each}
      {:else}
        <div class="empty-state">
          <div class="empty-icon"><Cloud size={36} strokeWidth={1.5} /></div>
          <p>{$t('game.cloud.empty')}</p>
          <p class="quiet">{$t('cloud.browser.emptyHint')}</p>
        </div>
      {/if}
    </div>
    <ModalFoot>
      <span class="quiet">{$t('cloud.browser.deleteFootnote')}</span>
      <button class="btn" on:click={close}>{$t('common.close')}</button>
    </ModalFoot>
  {:else if cloudGames}
    <!-- tile grid -->
    <div class="toolbar">
      <SearchInput placeholder={$t('cloud.browser.filterPlaceholder')} bind:value={filter} />
      <FilterTabs options={tabOptions} counts={tabCounts} bind:value={tab} />
    </div>

    <div class="list">
      <div class="grid">
        {#each filtered as tile (tile.id)}
          <CoverTile
            name={tile.name}
            src={tile.coverUrl}
            icon={tile.cloud ? Cloud : HardDrive}
            badge={tile.cloud ? String(tile.cloud.count) : $t('cloud.browser.badgeLocalOnly')}
            badgeIcon={tile.cloud ? Cloud : null}
            badgeAccent={!!tile.cloud}
            hoverLayout="center"
            on:activate={() => (detailId = tile.id)}
          >
            <svelte:fragment slot="hover">
              <button class="btn small primary" on:click|stopPropagation={() => (detailId = tile.id)}>
                {tile.cloud ? $t('game.header.browse') : $t('cloud.browser.upload')}
              </button>
            </svelte:fragment>
          </CoverTile>
        {:else}
          <div class="grid-empty">
            {tiles.length === 0 ? $t('cloud.browser.emptyAll') : $t('cloud.browser.emptyNoMatch')}
          </div>
        {/each}
      </div>
    </div>

    <ModalFoot>
      <span class="quiet">{$t('cloud.browser.gridHint')}</span>
      <button class="btn" on:click={close}>{$t('common.close')}</button>
    </ModalFoot>
  {/if}
</Modal>

<style>
  .quiet {
    color: var(--text-faint);
    font-size: 0.85rem;
  }
  .toolbar {
    display: flex;
    gap: 12px;
    padding: 14px 22px 10px;
    align-items: center;
    flex-wrap: wrap;
  }
  .list {
    flex: 1;
    overflow-y: auto;
    padding: 4px 22px 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 16px;
  }
  .grid-empty {
    grid-column: 1 / -1;
    text-align: center;
    color: var(--text-faint);
    padding: 50px 20px;
  }
  .detail-head {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 14px 22px;
    border-bottom: 1px solid var(--border);
  }
  .detail-title {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }
  .detail-title strong {
    font-size: 0.98rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .info {
    flex: 1;
  }
  .name {
    font-weight: 600;
    font-size: 0.9rem;
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .meta {
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  .empty-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 6px;
    color: var(--text-dim);
    text-align: center;
    padding: 20px;
  }
  .empty-icon {
    display: flex;
    justify-content: center;
    color: var(--text-faint);
  }
</style>
