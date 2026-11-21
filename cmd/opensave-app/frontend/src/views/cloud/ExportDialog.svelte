<script>
  // Export: tracked games plus saves the scan found, pre-ticked for the
  // tracked ones. The chosen games' LIVE saves, with where each belongs, go
  // into one .sscb file. Fires `close`.
  //
  // Where the file goes: the system's save dialog in the app, when Export is
  // pressed. The browser build has none, so there the dialog has a Save to
  // field with the folder navigator; it starts in Documents.
  import { createEventDispatcher, onDestroy, onMount } from 'svelte';
  import { toast, backupProgressEvent } from '../../lib/stores.js';
  import { api, native, coverURL } from '../../lib/api.js';
  import { t } from '../../lib/i18n.js';
  import Modal from '../../components/ui/Modal.svelte';
  import FileDown from 'lucide-svelte/icons/file-down';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import FolderBrowser from '../../components/FolderBrowser.svelte';
  import { collapse } from '../../lib/motion.js';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import ModalLoading from '../../components/ui/ModalLoading.svelte';
  import CoverTile from '../../components/ui/CoverTile.svelte';
  import ProgressBar from '../../components/ui/ProgressBar.svelte';

  const dispatch = createEventDispatcher();
  const close = () => dispatch('close');

  let items = null; // [{id,name,savePath,appId,cover,tracked}], null while loading
  let sel = {};
  let exporting = false;

  const app = native.isWails();
  const FILE = 'opensave-saves.sscb';
  let dest = '';
  let browsing = false;
  const sepOf = (dir) => (dir.includes('\\') ? '\\' : '/');
  const join = (dir, name) => (dir.endsWith('\\') || dir.endsWith('/') ? dir + name : dir + sepOf(dir) + name);
  const nameOf = (path) => path.split(/[\\/]/).pop() || FILE;
  const folderOf = (path) => path.slice(0, Math.max(0, path.length - nameOf(path).length - 1));
  onMount(async () => {
    if (app) return;
    try {
      const { places = [] } = await api.get('/api/folders');
      const start = places.find((p) => p.name === 'documents') ?? places[0];
      if (start && !dest) dest = join(start.path, FILE);
    } catch {
      // Typed in, then.
    }
  });

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
    let target = app ? await native.selectSaveFile($t('cloud.export.selectFileDialog'), FILE) : dest.trim();
    if (!target) return;
    if (!/\.sscb$/i.test(target)) target += '.sscb';
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
      let msg = $t('cloud.export.result', { n: res.exported });
      if (fromSnap) {
        msg += $t('cloud.export.fromSnap', {
          games: fromSnap === 1 ? res.fromSnapshot[0].name : $t('cloud.export.gamesCount', { n: fromSnap })
        });
      }
      if (skipped) msg += $t('cloud.export.skippedNote', { n: skipped });
      toast(msg, skipped ? 'info' : 'success');
      close();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      exporting = false;
    }
  }
</script>

<Modal title={$t('cloud.export.title')} icon={FileDown} onClose={close} closable={!exporting} width={920} height="min(84vh, 820px)">
  <svelte:fragment slot="sub">
    {#if !items}
      {$t('cloud.export.looking')}
    {:else}
      {$t('cloud.export.selected', { count, total: items.length })}
    {/if}
  </svelte:fragment>

  {#if !items}
    <ModalLoading>{$t('cloud.export.listing')}</ModalLoading>
  {:else}
    <div class="toolbar">
      <button class="btn small" on:click={() => setAll(!allSelected)}>
        {allSelected ? $t('home.library.unselectAll') : $t('cloud.export.selectAll')}
      </button>
      <button class="btn small ghost" on:click={() => { setAll(false); setAll(true, true); }}>{$t('cloud.export.trackedOnly')}</button>
    </div>
    <div class="list">
      {#if items.length === 0}
        <div class="empty-state">
          <div class="empty-icon"><FileDown size={36} strokeWidth={1.5} /></div>
          <p>{$t('cloud.export.empty')}</p>
        </div>
      {:else}
        <div class="grid">
          {#each items as it, i (it.id)}
            <div class="cell" style="--i: {Math.min(i, 16)}">
              <CoverTile
                name={it.name}
                title={it.savePath}
                src={it.cover}
                selected={!!sel[it.id]}
                badge={it.tracked ? $t('home.scan.tracked') : $t('cloud.export.badgeDetected')}
                on:activate={() => (sel[it.id] = !sel[it.id])}
              />
            </div>
          {/each}
        </div>
      {/if}
    </div>
    {#if !app}
      <div class="dest">
        <label for="export-dest">{$t('cloud.export.saveTo')}</label>
        <div class="dest-row">
          <input id="export-dest" bind:value={dest} spellcheck="false" />
          <button class="btn small" class:on={browsing} on:click={() => (browsing = !browsing)}><FolderOpen size={14} />{$t('folders.browse')}</button>
        </div>
        {#if browsing}
          <div transition:collapse>
            <FolderBrowser
              start={folderOf(dest)}
              on:pick={(e) => {
                dest = join(e.detail, nameOf(dest));
                browsing = false;
              }}
            />
          </div>
        {/if}
      </div>
    {/if}
    {#if exporting && prog}
      <ProgressBar done={prog.done} total={prog.total}>
        {$t('cloud.export.progress', { done: Math.min(prog.done + 1, prog.total), total: prog.total })}
        {#if prog.current}&nbsp;— <code>{prog.current}</code>{/if}
      </ProgressBar>
    {/if}
    <ModalFoot>
      <span class="quiet">{$t('cloud.export.detectedNote')}</span>
      <div class="actions">
        <button class="btn" disabled={exporting} on:click={close}>{$t('common.cancel')}</button>
        <button class="btn primary" disabled={exporting || count === 0 || (!app && !dest.trim())} on:click={run}>
          {exporting ? $t('cloud.export.exporting') : $t('cloud.export.exportCount', { n: count })}
        </button>
      </div>
    </ModalFoot>
  {/if}
</Modal>

<style>
  .quiet {
    color: var(--text-faint);
    font-size: var(--text-md);
  }
  .toolbar {
    flex-shrink: 0;
    display: flex;
    gap: 8px;
    padding: 14px 22px 4px;
  }
  .list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 8px 22px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(132px, 1fr));
    gap: 16px;
    padding: 4px;
  }
  /* The covers arrive one after another, moving only. */
  .cell {
    animation: arrive 0.34s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 22ms);
  }
  .dest {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px 22px 14px;
    border-top: 1px solid var(--border);
  }
  .dest label {
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  .dest-row {
    display: flex;
    gap: 8px;
  }
  .dest-row input {
    flex: 1;
    min-width: 0;
  }
  .btn.on {
    border-color: var(--accent);
    color: var(--accent-text);
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
