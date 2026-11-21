<script>
  // Import a .sscb backup. "snapshots" (the default) never touches live
  // files; "overwrite" restores every save in the file onto disk — the daemon
  // takes safety copies of anything it replaces. Fires `close`.
  //
  // The file is chosen here: the system's picker in the app (opened straight
  // away when no file came with the dialog), or the folder navigator in the
  // browser build, which has no picker of its own.
  import { createEventDispatcher, onDestroy, onMount } from 'svelte';
  import { toast, backupProgressEvent } from '../../lib/stores.js';
  import { api, native } from '../../lib/api.js';
  import { t } from '../../lib/i18n.js';
  import { collapse } from '../../lib/motion.js';
  import Modal from '../../components/ui/Modal.svelte';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import ProgressBar from '../../components/ui/ProgressBar.svelte';
  import FolderBrowser from '../../components/FolderBrowser.svelte';
  import ArchiveRestore from 'lucide-svelte/icons/archive-restore';
  import FileArchive from 'lucide-svelte/icons/file-archive';
  import History from 'lucide-svelte/icons/history';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import ShieldCheck from 'lucide-svelte/icons/shield-check';

  /** The backup file, when one was chosen before the dialog opened. */
  export let source = '';

  const dispatch = createEventDispatcher();
  const close = () => dispatch('close');
  const app = native.isWails();

  let mode = 'snapshots';
  let importing = false;
  let browsing = false;

  // Live per-game progress while the daemon walks the import.
  let prog = null;
  const unsubProg = backupProgressEvent.subscribe((ev) => {
    prog = ev && !ev.complete ? ev : null;
  });
  onDestroy(unsubProg);

  async function choose() {
    if (!app) {
      browsing = !browsing;
      return;
    }
    const picked = await native.selectBackupFile($t('cloud.backup.selectImportFile'));
    if (picked) source = picked;
  }
  onMount(() => {
    if (!source) choose();
  });

  $: fileName = source.split(/[\\/]/).pop();
  $: folder = source.slice(0, Math.max(0, source.length - fileName.length - 1));

  async function run() {
    if (!source) return;
    importing = true;
    try {
      const res = await api.post('/api/backup/restore', { sourcePath: source, mode });
      if (res.legacy) {
        toast($t('cloud.import.legacyResult', { n: res.imported, skipped: res.skipped }), 'success');
      } else {
        const bits = [];
        if (res.restored) bits.push($t('cloud.import.restoredBits', { n: res.restored }));
        if (res.snapshots) bits.push($t('cloud.import.addedBits', { n: res.snapshots }));
        if (res.skipped) bits.push($t('cloud.import.skippedBits', { n: res.skipped }));
        toast($t('cloud.import.finished', { details: bits.join(', ') || $t('cloud.import.nothing') }), res.skipped ? 'info' : 'success');
      }
      close();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      importing = false;
    }
  }

  const MODES = [
    { id: 'snapshots', icon: History, title: 'cloud.import.addToSnapshots', desc: 'cloud.import.snapshotsDesc', tag: 'cloud.import.recommended' },
    { id: 'overwrite', icon: TriangleAlert, title: 'cloud.import.overwriteTitle', desc: 'cloud.import.overwriteDesc', tag: '' }
  ];
</script>

<Modal title={$t('cloud.import.title')} icon={ArchiveRestore} onClose={close} closable={!importing} width={600} height="auto" maxHeight="min(86vh, 760px)">
  <svelte:fragment slot="sub">{$t('cloud.import.sub')}</svelte:fragment>

  <div class="body">
    <div class="file" class:none={!source}>
      <span class="ftile"><FileArchive size={20} /></span>
      <span class="ftext">
        {#if source}
          <b title={source}>{fileName}</b>
          <span class="fpath" title={folder}><bdi>{folder}</bdi></span>
        {:else}
          <b>{$t('cloud.import.noFile')}</b>
          <span class="fpath">{$t('cloud.import.noFileSub')}</span>
        {/if}
      </span>
      <button class="btn small" class:primary={!source} class:on={browsing} disabled={importing} on:click={choose}>
        {source ? $t('cloud.import.chooseAnother') : $t('cloud.import.chooseFile')}
      </button>
    </div>
    {#if browsing}
      <div transition:collapse>
        <FolderBrowser
          files=".sscb"
          start={source}
          on:pick={(e) => {
            source = e.detail;
            browsing = false;
          }}
        />
      </div>
    {/if}

    <div class="label" id="import-mode">{$t('cloud.import.whatHappens')}</div>
    <div class="modes" role="radiogroup" aria-labelledby="import-mode">
      {#each MODES as m}
        <button
          class="mode m-{m.id}"
          class:selected={mode === m.id}
          role="radio"
          aria-checked={mode === m.id}
          disabled={importing}
          on:click={() => (mode = m.id)}
        >
          <span class="mtile"><svelte:component this={m.icon} size={18} /></span>
          <span class="mtext">
            <span class="mtitle">{$t(m.title)}{#if m.tag}<span class="tag">{$t(m.tag)}</span>{/if}</span>
            <span class="mdesc">{$t(m.desc)}</span>
          </span>
          <span class="radio" aria-hidden="true"></span>
        </button>
      {/each}
    </div>

    {#if importing && prog}
      <ProgressBar done={prog.done} total={prog.total}>
        {$t('cloud.import.progress', { done: Math.min(prog.done + 1, prog.total), total: prog.total })}
        {#if prog.current}&nbsp;— <code>{prog.current}</code>{/if}
      </ProgressBar>
    {/if}
  </div>

  <ModalFoot>
    <span class="note"><ShieldCheck size={15} />{$t('cloud.import.safetyNote')}</span>
    <div class="actions">
      <button class="btn" disabled={importing} on:click={close}>{$t('common.cancel')}</button>
      <button class="btn {mode === 'overwrite' ? 'danger' : 'primary'}" disabled={importing || !source} on:click={run}>
        {importing ? $t('cloud.import.importing') : mode === 'overwrite' ? $t('cloud.import.overwriteBtn') : $t('cloud.import.addToSnapshots')}
      </button>
    </div>
  </ModalFoot>
</Modal>

<style>
  /* Scrolls inside the dialog when the window is short; its parts keep their
     height rather than being squeezed. */
  .body {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    padding: 18px 22px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .body > :global(*) {
    flex-shrink: 0;
  }
  .file {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px 10px 10px;
    border-radius: 14px;
    background: var(--bg);
    border: 1px solid var(--border);
  }
  .file.none {
    border-style: dashed;
    border-color: var(--border-strong);
  }
  .ftile {
    width: 42px;
    height: 42px;
    flex: none;
    border-radius: 11px;
    display: grid;
    place-items: center;
    background: rgba(201, 162, 74, 0.16);
    color: #c9a24a;
  }
  .none .ftile {
    background: var(--bg-active);
    color: var(--text-faint);
  }
  .ftext {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .ftext b {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  /* The end of a path names the folder, so a long one keeps its end. */
  .fpath {
    font-size: var(--text-sm);
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl;
    text-align: left;
  }
  /* Open: marked on a plain button; a filled one is marked enough. */
  .btn.on:not(.primary) {
    border-color: var(--accent);
    color: var(--accent-text);
  }
  .label {
    margin-top: 4px;
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  .modes {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .mode {
    display: flex;
    align-items: flex-start;
    gap: 14px;
    padding: 14px 16px 14px 14px;
    border-radius: 14px;
    border: 1.5px solid var(--border);
    background: var(--bg);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color 0.18s,
      background 0.18s,
      transform 0.18s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .mode:hover:not(:disabled) {
    border-color: var(--border-strong);
    transform: translateY(-1px);
  }
  .mode.selected {
    border-color: var(--accent);
    background: linear-gradient(0deg, var(--accent-soft), var(--accent-soft)), var(--bg);
  }
  .m-overwrite.selected {
    border-color: rgba(var(--danger-rgb), 0.8);
    background: linear-gradient(0deg, rgba(var(--danger-rgb), 0.1), rgba(var(--danger-rgb), 0.1)), var(--bg);
  }
  .mtile {
    width: 38px;
    height: 38px;
    flex: none;
    border-radius: 11px;
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--text-dim);
    transition:
      background 0.18s,
      color 0.18s;
  }
  .selected .mtile {
    background: rgba(var(--accent-rgb), 0.22);
    color: var(--accent-text);
  }
  .m-overwrite.selected .mtile {
    background: rgba(var(--danger-rgb), 0.18);
    color: var(--danger-text);
  }
  .mtext {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .mtitle {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
  }
  .tag {
    padding: 1px 8px;
    border-radius: 999px;
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--success);
    background: rgba(var(--success-rgb), 0.14);
  }
  .mdesc {
    font-size: var(--text-md);
    color: var(--text-dim);
    line-height: 1.45;
  }
  /* The radio, drawn: a ring, and a dot that grows into it when chosen. */
  .radio {
    position: relative;
    width: 18px;
    height: 18px;
    flex: none;
    margin-top: 2px;
    border-radius: 50%;
    border: 2px solid var(--border-strong);
    transition: border-color 0.18s;
  }
  .radio::after {
    content: '';
    position: absolute;
    inset: 3px;
    border-radius: 50%;
    background: var(--accent);
    transform: scale(0);
    transition: transform 0.2s cubic-bezier(0.3, 1.4, 0.5, 1);
  }
  .selected .radio {
    border-color: var(--accent);
  }
  .selected .radio::after {
    transform: scale(1);
  }
  .m-overwrite.selected .radio {
    border-color: var(--danger);
  }
  .m-overwrite .radio::after {
    background: var(--danger);
  }
  .note {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    font-size: var(--text-md);
    color: var(--text-faint);
  }
  .note :global(svg) {
    color: var(--success);
    flex: none;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
</style>
