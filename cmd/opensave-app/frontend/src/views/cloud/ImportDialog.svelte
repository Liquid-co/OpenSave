<script>
  // Import a .sscb backup. "snapshots" (the default) never touches live
  // files; "overwrite" restores every save in the file onto disk — the daemon
  // takes safety copies of anything it replaces. Fires `close`.
  import { createEventDispatcher, onDestroy } from 'svelte';
  import { toast, backupProgressEvent } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import Modal from '../../components/ui/Modal.svelte';
  import PackageOpen from 'lucide-svelte/icons/package-open';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import ProgressBar from '../../components/ui/ProgressBar.svelte';

  /** The backup file chosen. */
  export let source = '';

  const dispatch = createEventDispatcher();
  const close = () => dispatch('close');

  let mode = 'snapshots';
  let importing = false;

  // Live per-game progress while the daemon walks the import.
  let prog = null;
  const unsubProg = backupProgressEvent.subscribe((ev) => {
    prog = ev && !ev.complete ? ev : null;
  });
  onDestroy(unsubProg);

  async function run() {
    importing = true;
    try {
      const res = await api.post('/api/backup/restore', { sourcePath: source, mode });
      if (res.legacy) {
        toast(`Imported ${res.imported} snapshot(s), skipped ${res.skipped}`, 'success');
      } else {
        const bits = [];
        if (res.restored) bits.push(`${res.restored} restored`);
        if (res.snapshots) bits.push(`${res.snapshots} added to snapshots`);
        if (res.skipped) bits.push(`${res.skipped} skipped`);
        toast(`Import finished: ${bits.join(', ') || 'nothing imported'} — details in Activity`, res.skipped ? 'info' : 'success');
      }
      close();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      importing = false;
    }
  }
</script>

<!-- Two choices and a footer don't need the full browser-modal height —
     sized to content so the dialog doesn't look hollow. -->
<Modal title="Import backup" icon={PackageOpen} onClose={close} closable={!importing} width={560} height="auto" maxHeight="min(78vh, 720px)">
  <svelte:fragment slot="sub"><code>{source}</code></svelte:fragment>

  <div class="modes">
    <label class="mode" class:selected={mode === 'snapshots'}>
      <input type="radio" bind:group={mode} value="snapshots" />
      <div>
        <div class="mode-title">Add to snapshots <span class="badge online">recommended</span></div>
        <p class="quiet">
          Each save in the file is added to its game's snapshot history. Nothing on disk
          changes — restore individual games whenever you choose. Games not tracked on this
          machine are reported in Activity and skipped.
        </p>
      </div>
    </label>
    <label class="mode danger-mode" class:selected={mode === 'overwrite'}>
      <input type="radio" bind:group={mode} value="overwrite" />
      <div>
        <div class="mode-title">Overwrite current saves</div>
        <p class="quiet">
          Every save in the file is written to its location on this machine — tracked games to
          their tracked folder, others to the path recorded in the backup. A safety copy of
          whatever is there now is taken first. Check Activity afterwards for exactly what was
          restored where.
        </p>
      </div>
    </label>
  </div>

  {#if importing && prog}
    <ProgressBar done={prog.done} total={prog.total}>
      Importing {Math.min(prog.done + 1, prog.total)} of {prog.total}
      {#if prog.current}&nbsp;— <code>{prog.current}</code>{/if}
    </ProgressBar>
  {/if}
  <ModalFoot>
    <span class="quiet">Nothing is ever overwritten without a safety copy.</span>
    <div class="actions">
      <button class="btn" disabled={importing} on:click={close}>Cancel</button>
      <button class="btn {mode === 'overwrite' ? 'danger' : 'primary'}" disabled={importing} on:click={run}>
        {importing ? 'Importing…' : mode === 'overwrite' ? 'Overwrite saves' : 'Add to snapshots'}
      </button>
    </div>
  </ModalFoot>
</Modal>

<style>
  .quiet {
    color: var(--text-faint);
    font-size: 0.85rem;
  }
  .modes {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 16px 20px;
  }
  .mode {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 14px;
    border: 1px solid var(--border);
    border-radius: 10px;
    cursor: pointer;
  }
  .mode input {
    margin-top: 3px;
  }
  .mode.selected {
    border-color: var(--accent);
    background: var(--bg-hover, rgba(128, 128, 128, 0.06));
  }
  .mode.danger-mode.selected {
    border-color: var(--danger, var(--danger));
  }
  .mode-title {
    font-weight: 600;
    margin-bottom: 4px;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .mode p {
    margin: 0;
    font-size: 0.82rem;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
</style>
