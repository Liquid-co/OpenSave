<script>
  import { t as translateUI } from '../../lib/i18n.js';
  // Export: tracked games plus saves the scan found, pre-ticked for the
  // tracked ones. The chosen games' LIVE saves, with where each belongs, go
  // into one .sscb file. Fires `close`.
  import { createEventDispatcher, onDestroy } from 'svelte';
  import { toast, backupProgressEvent } from '../../lib/stores.js';
  import { api, native, coverURL } from '../../lib/api.js';
  import Modal from '../../components/ui/Modal.svelte';
  import Package from 'lucide-svelte/icons/package';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import ModalLoading from '../../components/ui/ModalLoading.svelte';
  import CoverTile from '../../components/ui/CoverTile.svelte';
  import ProgressBar from '../../components/ui/ProgressBar.svelte';

  const dispatch = createEventDispatcher();
  const close = () => dispatch('close');

  let items = null; // [{id,name,savePath,appId,cover,tracked}], null while loading
  let sel = {};
  let exporting = false;

  // Live per-game progress while the daemon walks the export.
  let prog = null;
  const unsubProg = backupProgressEvent.subscribe((ev) => {
    prog = ev && !ev.complete ? ev : null;
  });
  onDestroy(unsubProg);

  // Portrait box art, through the daemon like every other cover. This screen
  // hotlinked Steam's CDN directly, which is the one thing the cover proxy
  // exists to avoid: the embedded webview cannot reliably reach an external
  // host, and on a network that blocks Steam these were the only blank tiles
  // left in the app. The daemon caches, falls back to an image proxy, and can
  // answer for a game with no App ID at all.
  const portraitUrl = (appId, name = '') => coverURL(appId, true, name);

  async function load() {
    try {
      const [games, scan] = await Promise.all([api.get('/api/games'), api.get('/api/presets/scan')]);
      const tracked = Object.values(games ?? {}).map((g) => ({
        id: g.id, name: g.name, savePath: g.savePath, appId: g.appId,
        // Portrait box art first (matches the scan tiles); the stored
        // coverUrl is Steam's landscape header — wrong shape for tiles.
        cover: portraitUrl(g.appId, g.name) || g.coverUrl || '', tracked: true
      }));
      const knownPaths = new Set(tracked.map((g) => g.savePath.toLowerCase()));
      const knownIds = new Set(tracked.map((g) => g.id));
      const detected = (scan ?? [])
        .filter((d) => !knownPaths.has(d.savePath.toLowerCase()) && !knownIds.has(d.id))
        // A folder holding no files has nothing to back up. `measured` guards
        // the case where the daemon could not read it — unknown is not empty,
        // and dropping one of those would quietly skip a real save.
        .filter((d) => !(d.measured && d.fileCount === 0))
        .map((d) => ({
          id: d.id, name: d.name, savePath: d.savePath, appId: d.appId,
          cover: portraitUrl(d.appId, d.name), tracked: false
        }));
      for (const g of tracked) sel[g.id] = true; // tracked pre-selected
      items = [...tracked, ...detected];
    } catch (e) {
      toast(e.message, 'error');
      close();
    }
  }
  load();

  const setAll = (value, trackedOnly = false) => {
    for (const it of items ?? []) sel[it.id] = trackedOnly ? value && it.tracked : value;
    sel = sel;
  };

  $: count = items ? items.filter((it) => sel[it.id]).length : 0;
  $: allSelected = !!items && items.length > 0 && count === items.length;

  async function run() {
    const chosen = (items ?? []).filter((it) => sel[it.id]);
    if (!chosen.length) return;
    const target = await native.selectSaveFile($translateUI('exportDialog.exportSelectedSaves'), 'opensave-saves.sscb');
    if (!target) return;
    exporting = true;
    try {
      const res = await api.post('/api/backup/export', {
        targetPath: target,
        games: chosen.map(({ id, name, appId, savePath }) => ({ id, name, appId, savePath }))
      });
      const skipped = res.skipped?.length ?? 0;
      // A game whose save folder was empty or gone went in as its newest
      // snapshot; say so, since that is not the save as it stands.
      const fromSnap = res.fromSnapshot?.length ?? 0;
      toast(
        $translateUI('exportDialog.exportedSave', { p0: res.exported, p1: res.exported === 1 ? '' : 's' }) +
          (fromSnap ? $translateUI('exportDialog.fromTheNewestSnapshotAs', { p0: fromSnap === 1 ? res.fromSnapshot[0].name : $translateUI('exportDialog.games', { p0: fromSnap }) }) : '') +
          (skipped ? $translateUI('exportDialog.skippedSeeActivity', { p0: skipped }) : ''),
        skipped ? 'info' : 'success'
      );
      close();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      exporting = false;
    }
  }
</script>

<Modal title="{$translateUI('exportDialog.exportSaves')}" icon={Package} onClose={close}>
  <svelte:fragment slot="sub">
    {#if !items}
      {$translateUI('exportDialog.lookingForSavesOnThisMachine')}
    {:else}
      {count} {$translateUI('cloudBrowser.of')} {items.length} {$translateUI('exportDialog.selectedEachGameSCurrentSaveIs')}
    {/if}
  </svelte:fragment>

  {#if !items}
    <ModalLoading>{$translateUI('exportDialog.listingTrackedGamesAndScanningForSaves')}</ModalLoading>
  {:else}
    <div class="toolbar">
      <button class="btn small" on:click={() => setAll(!allSelected)}>
        {allSelected ? $translateUI('home.library.unselectAll') : $translateUI('exportDialog.selectAll')}
      </button>
      <button class="btn small" on:click={() => { setAll(false); setAll(true, true); }}>{$translateUI('exportDialog.trackedOnly')}</button>
    </div>
    <div class="list">
      {#if items.length === 0}
        <div class="empty-state">
          <div class="empty-icon"><Package size={36} strokeWidth={1.5} /></div>
          <p>{$translateUI('exportDialog.noSavesFoundToExport')}</p>
        </div>
      {:else}
        <div class="grid">
          {#each items as it (it.id)}
            <CoverTile
              name={it.name}
              title={it.savePath}
              src={it.cover}
              selected={!!sel[it.id]}
              badge={it.tracked ? 'Tracked' : 'Detected'}
              on:activate={() => (sel[it.id] = !sel[it.id])}
            />
          {/each}
        </div>
      {/if}
    </div>
    {#if exporting && prog}
      <ProgressBar done={prog.done} total={prog.total}>
        {$translateUI('exportDialog.exporting')} {Math.min(prog.done + 1, prog.total)} {$translateUI('cloudBrowser.of')} {prog.total}
        {#if prog.current}&nbsp;— <code>{prog.current}</code>{/if}
      </ProgressBar>
    {/if}
    <ModalFoot>
      <span class="quiet">{$translateUI('exportDialog.detectedSavesExportFineWithoutBeingTracked')}</span>
      <div class="actions">
        <button class="btn" on:click={close}>{$translateUI('common.cancel')}</button>
        <button class="btn primary" disabled={exporting || count === 0} on:click={run}>
          {exporting ? $translateUI('exportDialog.exportingMessage') : $translateUI('exportDialog.exportSave', { p0: count, p1: count === 1 ? '' : 's' })}
        </button>
      </div>
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
    gap: 8px;
    padding: 10px 20px 0;
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
    padding: 4px;
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
    color: var(--text-faint);
  }
  .actions {
    display: flex;
    gap: 8px;
  }
</style>
