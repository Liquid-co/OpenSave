<script>
  // One game's copies in the cloud, newest first, marked like the snapshot
  // rows: why each was taken, when, and its size. Put one back, delete it
  // from the cloud, or send up what is missing. Fires `close`, `putBack`
  // (with the copy), `upload` and `deleted`.
  //
  // Sized to what it holds, up to most of the window, and the list scrolls
  // inside it: one copy is a short dialog, two hundred a scrolling one.
  // The rows arrive one after another (moving only), and a deleted one
  // closes up rather than vanishing.
  import { createEventDispatcher } from 'svelte';
  import { collapse } from '../../lib/motion.js';
  import Cloud from 'lucide-svelte/icons/cloud';
  import ArrowUp from 'lucide-svelte/icons/arrow-up';
  import Trash2 from 'lucide-svelte/icons/trash-2';
  import Modal from '../../components/ui/Modal.svelte';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import { api } from '../../lib/api.js';
  import { toast, askConfirm } from '../../lib/stores.js';
  import { fmtSize } from '../../lib/format.js';
  import { snapshotKind, dayLabel } from '../../lib/snapshots.js';
  import { copyKind, copyTime, gameState } from '../../lib/backupsview.js';
  import { t } from '../../lib/i18n.js';

  export let game;
  export let busy = false;
  /** This game's upload is the one running. */
  export let uploadingThis = false;
  const dispatch = createEventDispatcher();
  const close = () => dispatch('close');

  $: copies = [...game.copies].sort((a, b) => copyTime(b) - copyTime(a));
  $: st = gameState(game);

  function describe(c) {
    const kind = copyKind(c);
    if (kind === 'unknown') return { kind, title: $t('backups.tip.unknownTitle') };
    return { kind, title: snapshotKind({ comment: c.comment ?? '', isSystemAuto: !!c.isSystemAuto }).title };
  }

  let deleting = '';
  async function remove(c) {
    const when = new Date(copyTime(c)).toLocaleString(undefined, { day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit' });
    if (!(await askConfirm($t('backups.deleteBody', { game: game.gameName, when }), { title: $t('backups.deleteTitle'), confirmText: $t('common.delete'), danger: true }))) return;
    deleting = c.name;
    try {
      await api.post(`/api/cloud/delete/${game.gameId}`, { fileName: c.name, id: c.id ?? '' });
      game = { ...game, copies: game.copies.filter((x) => x.name !== c.name), totalSize: game.totalSize - c.sizeBytes };
      toast($t('cloud.browser.deletedToast'), 'success');
      dispatch('deleted');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      deleting = '';
    }
  }
</script>

<Modal title={game.gameName} icon={Cloud} width={640} height="auto" maxHeight="min(86vh, 780px)" onClose={close}>
  <svelte:fragment slot="sub">
    {#if copies.length}{$t('backups.copies', { n: copies.length, size: fmtSize(game.totalSize) })}{:else}{$t('backups.noCopies')}{/if}
  </svelte:fragment>
  <svelte:fragment slot="actions">
    {#if game.tracked}
      <button class="btn small" class:sending={uploadingThis} disabled={busy} on:click={() => dispatch('upload')}><span class="arrow"><ArrowUp size={14} /></span>{uploadingThis ? $t('backups.sending') : $t('backups.uploadMissing')}</button>
    {/if}
  </svelte:fragment>

  <div class="body">
    {#if st === 'behind'}<p class="note info">{$t('backups.behindNote')}</p>{/if}
    {#if !game.tracked}<p class="note">{$t('backups.notTrackedNote')}</p>{/if}
    {#if copies.length}
      <div class="rows">
        {#each copies as c, i (c.name)}
          {@const d = describe(c)}
          {@const ms = copyTime(c)}
          {#if i === 0 || dayLabel(ms) !== dayLabel(copyTime(copies[i - 1]))}
            <div class="day" style="--i: {Math.min(i, 12)}">{dayLabel(ms) === 'Today' ? $t('backups.today') : dayLabel(ms) === 'Yesterday' ? $t('backups.yesterday') : dayLabel(ms)}</div>
          {/if}
          <div class="row k-{d.kind}" style="--i: {Math.min(i, 12)}" out:collapse={{ duration: 220 }}>
            <span class="time">{new Date(ms).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}</span>
            <span class="info">
              <span class="title">{d.title}</span>
              <span class="meta">
                <span class="tag">{$t('backups.kind.' + d.kind)}</span>
                <span>{fmtSize(c.sizeBytes)}</span>
                {#if c.branch && c.branch !== 'main'}<span>{$t('backups.onBranch', { branch: c.branch })}</span>{/if}
                {#if c.snapshotId === game.current}<span class="tag current">{$t('backups.currentSave')}</span>{/if}
              </span>
            </span>
            <button class="btn small" disabled={!game.tracked} title={game.tracked ? '' : $t('backups.trackFirst')} on:click={() => dispatch('putBack', c)}>{$t('backups.putBack')}</button>
            <button class="btn small ghost danger-ghost" aria-label={$t('common.delete')} title={$t('backups.deleteHint')} disabled={!!deleting} on:click={() => remove(c)}><Trash2 size={14} /></button>
          </div>
        {/each}
      </div>
    {:else}
      <p class="empty">{st === 'waiting' ? $t('backups.waitingNote', { n: game.localSnapshots }) : $t('backups.noCopies')}</p>
    {/if}
  </div>
  <ModalFoot>
    <span class="foot-note">{$t('cloud.browser.deleteFootnote')}</span>
    <button class="btn" on:click={close}>{$t('common.close')}</button>
  </ModalFoot>
</Modal>

<style>
  /* Takes what the dialog has left and scrolls; its parts keep their own
     height. The list used to be squeezed instead: with overflow hidden it
     shrank to fit and cut its last rows off, and nothing scrolled. */
  .body {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    padding: 14px 22px 18px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .body > * {
    flex-shrink: 0;
  }
  .day,
  .row {
    animation: arrive 0.3s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 25ms);
  }
  .arrow {
    display: grid;
    overflow: hidden;
  }
  .sending .arrow :global(svg) {
    animation: send 0.9s cubic-bezier(0.5, 0, 0.5, 1) infinite;
  }
  @keyframes send {
    45% {
      transform: translateY(-16px);
    }
    46% {
      transform: translateY(16px);
    }
  }
  .note {
    font-size: var(--text-md);
    color: var(--text-dim);
    padding: 10px 12px;
    border-radius: 10px;
    background: var(--bg);
    border: 1px solid var(--border);
  }
  .note.info {
    border-color: rgba(var(--info-rgb), 0.4);
  }
  .rows {
    display: flex;
    flex-direction: column;
    border-radius: 12px;
    border: 1px solid var(--border);
    overflow: hidden;
  }
  .day {
    padding: 8px 12px 6px;
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-faint);
    background: var(--bg);
    border-top: 1px solid var(--border);
  }
  .day:first-child {
    border-top: 0;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 9px 12px;
    border-top: 1px solid var(--border);
    border-left: 3px solid transparent;
  }
  .row:hover {
    background: var(--bg-hover);
  }
  .row.k-session {
    border-left-color: var(--success);
  }
  .row.k-manual {
    border-left-color: var(--accent);
  }
  .row.k-safety {
    border-left-color: var(--warn);
  }
  .time {
    width: 70px;
    flex: none;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .title {
    font-size: var(--text-md);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .meta {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .tag {
    border: 1px solid var(--border-strong);
    border-radius: 999px;
    padding: 0 7px;
    line-height: 1.6;
    color: var(--text-dim);
    white-space: nowrap;
  }
  .k-session .tag:first-child {
    border-color: rgba(var(--success-rgb), 0.45);
    color: var(--success);
  }
  .k-manual .tag:first-child {
    border-color: var(--accent);
    color: var(--accent-text);
  }
  .k-safety .tag:first-child {
    border-color: rgba(var(--warn-rgb), 0.45);
    color: var(--warn);
  }
  .k-unknown .tag:first-child {
    border-style: dashed;
  }
  .tag.current {
    border-color: rgba(var(--success-rgb), 0.45);
    color: var(--success);
  }
  .danger-ghost:hover:not(:disabled) {
    color: var(--danger-text);
  }
  .empty {
    color: var(--text-dim);
    padding: 18px 0;
  }
  .foot-note {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
</style>
