<script>
  // The files inside one snapshot, each with a way to put just that file
  // back. Fires `close`.
  import { createEventDispatcher, onMount } from 'svelte';
  import { askConfirm, toast } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import { fmtSize } from '../../lib/format.js';
  import { whenLabel } from '../../lib/snapshots.js';
  import { t } from '../../lib/i18n.js';
  import Modal from '../../components/ui/Modal.svelte';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import ModalLoading from '../../components/ui/ModalLoading.svelte';
  import SearchInput from '../../components/ui/SearchInput.svelte';
  import FileText from 'lucide-svelte/icons/file-text';
  import FolderArchive from 'lucide-svelte/icons/folder-archive';

  export let game;
  export let runner;
  export let snap;
  const { busy, run } = runner;
  const dispatch = createEventDispatcher();
  const close = () => dispatch('close');

  let files = null;
  onMount(async () => {
    try {
      files = (await api.get(`/api/games/${game.id}/snapshot/${snap.id}/files`)).filter((f) => !f.isDir);
    } catch (e) {
      toast(e.message, 'error');
      close();
    }
  });
  let query = '';
  $: shown = (files ?? []).filter((f) => !query.trim() || f.path.toLowerCase().includes(query.trim().toLowerCase()));
  $: total = (files ?? []).reduce((n, f) => n + (f.size ?? 0), 0);
  $: named = $t('game.snapshots.named', { when: whenLabel(snap.timestamp) });

  const restoreFile = async (relPath) => {
    if (!(await askConfirm($t('game.snapshots.fileRestoreConfirm', { path: relPath, snap: named }), { title: $t('game.snapshots.fileRestoreTitle'), confirmText: $t('game.snapshots.restore') }))) return;
    return run($t('game.snapshots.fileRestored', { path: relPath }), () => api.post(`/api/games/${game.id}/snapshot/${snap.id}/restore-file`, { relPath }));
  };
</script>

<Modal title={$t('game.snapshots.filesIn', { snap: named })} icon={FolderArchive} width={640} height="auto" maxHeight="min(86vh, 760px)" onClose={close}>
  <svelte:fragment slot="sub">{files ? $t('game.history.filesSub', { n: files.length, size: fmtSize(total) }) : ''}</svelte:fragment>
  {#if !files}
    <ModalLoading>{$t('game.history.readingFiles')}</ModalLoading>
  {:else}
    <div class="body">
      {#if files.length > 8}<SearchInput placeholder={$t('game.history.findFile')} bind:value={query} />{/if}
      <div class="files">
        {#each shown as f, i (f.path)}
          <div class="file" style="--i: {Math.min(i, 14)}">
            <FileText size={15} />
            <span class="path" title={f.path}><bdi>{f.path}</bdi></span>
            <span class="size">{fmtSize(f.size)}</span>
            <button class="btn small" disabled={$busy} on:click={() => restoreFile(f.path)}>{$t('game.snapshots.restoreFile')}</button>
          </div>
        {:else}
          <p class="none">{$t('game.history.noFiles')}</p>
        {/each}
      </div>
    </div>
  {/if}
  <ModalFoot>
    <span class="note">{$t('game.history.fileNote')}</span>
    <button class="btn" on:click={close}>{$t('common.close')}</button>
  </ModalFoot>
</Modal>

<style>
  .body {
    padding: 14px 22px 18px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .files {
    display: flex;
    flex-direction: column;
    border-radius: 12px;
    border: 1px solid var(--border);
  }
  .file {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px 8px 12px;
    border-top: 1px solid var(--border);
    font-size: var(--text-md);
    animation: arrive 0.28s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 18ms);
  }
  .file:first-child {
    border-top: 0;
  }
  .file:hover {
    background: var(--bg-hover);
  }
  .file > :global(svg) {
    flex: none;
    color: var(--text-faint);
  }
  /* The end of a path names the file, so a long one keeps its end. */
  .path {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl;
    text-align: left;
  }
  .size {
    font-size: var(--text-sm);
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }
  .none {
    padding: 14px;
    color: var(--text-dim);
  }
  .note {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
</style>
