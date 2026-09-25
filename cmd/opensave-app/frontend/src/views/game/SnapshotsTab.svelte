<script>
  // Every snapshot of this game, newest first and grouped by day: take one,
  // restore one, look inside one and put back a single file.
  import { onDestroy, tick as rendered } from 'svelte';
  import { askConfirm, toast } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import { fmtSize } from '../../lib/format.js';
  import { groupByDay, snapshotKind, whenLabel } from '../../lib/snapshots.js';
  import { askRestore } from '../../lib/restore.js';
  import { timeAgo } from '../../lib/timeago.js';
  import Camera from 'lucide-svelte/icons/camera';
  import Pin from 'lucide-svelte/icons/pin';
  import PinOff from 'lucide-svelte/icons/pin-off';
  import Pencil from 'lucide-svelte/icons/pencil';

  export let game;
  export let runner;
  /** A snapshot to bring into view and mark, when the page was opened on it. */
  export let focus = null;
  const { busy, run } = runner;

  let flashing = null;
  let listEl;
  $: if (focus && listEl) showSnapshot(focus);
  async function showSnapshot(id) {
    await rendered();
    const row = listEl?.querySelector(`[data-snap="${CSS.escape(id)}"]`);
    if (!row) return;
    row.scrollIntoView({ block: 'center', behavior: 'smooth' });
    flashing = id;
    setTimeout(() => {
      if (flashing === id) flashing = null;
    }, 1800);
  }

  let comment = '';
  let browsing = null; // {snap, files}

  // Times are relative to now, and now moves.
  let now = new Date();
  const tick = setInterval(() => (now = new Date()), 30_000);
  onDestroy(() => clearInterval(tick));

  $: branchCount = Object.keys(game.branches ?? {}).length;
  $: allSnapshots = Object.values(game.branches ?? {})
    .flatMap((b) => (b.snapshots ?? []).map((s) => ({ ...s, branch: b.name })))
    .sort((a, b) => (a.timestamp < b.timestamp ? 1 : -1));
  $: groups = groupByDay(allSnapshots, now);
  // The newest snapshot on the branch being played: what a restore of
  // "the last good save" would most often reach for.
  $: latestId = allSnapshots.find((s) => s.branch === game.activeBranch)?.id;

  // How a snapshot is named in a question about it.
  const named = (snap) => `the snapshot from ${whenLabel(snap.timestamp, now)}`;

  const takeSnapshot = () =>
    run('Snapshot created', async () => {
      await api.post(`/api/games/${game.id}/snapshot`, { comment });
      comment = '';
    });

  // Asked with what it would change; see lib/restore.js.
  const rollback = async (snap) => {
    if (!(await askRestore(game, snap))) return;
    return run('Snapshot restored', () => api.post(`/api/games/${game.id}/rollback`, { snapshotId: snap.id }));
  };

  // Pinned: no limit, age rule or clean-up removes it. Not a lock — deleting
  // it by hand still works, and says so first.
  const togglePin = (snap) =>
    run(snap.pinned ? 'Unpinned — the limits apply to it again' : 'Pinned — kept through every limit and clean-up', () =>
      api.patch(`/api/games/${game.id}/snapshot/${snap.id}`, { pinned: !snap.pinned })
    );

  // The note is edited in place; Enter or leaving the field saves it,
  // Escape puts it back.
  let noting = null; // snapshot id being noted
  let noteDraft = '';
  async function startNote(snap) {
    noting = snap.id;
    noteDraft = snap.note ?? '';
    await rendered();
    document.getElementById(`note-${snap.id}`)?.focus();
  }
  async function saveNote(snap) {
    if (noting !== snap.id) return;
    noting = null;
    if (noteDraft.trim() === (snap.note ?? '')) return;
    await run('', () => api.patch(`/api/games/${game.id}/snapshot/${snap.id}`, { note: noteDraft }));
  }
  function noteKeydown(e, snap) {
    if (e.key === 'Enter') saveNote(snap);
    else if (e.key === 'Escape') {
      e.stopPropagation();
      noting = null;
    }
  }

  async function deleteSnapshot(snap) {
    const pinnedLine = snap.pinned ? ' It is pinned, so nothing else would ever have removed it.' : '';
    if (!(await askConfirm(`Delete ${named(snap)}?${pinnedLine} This can't be undone; your current save isn't affected.`, { title: 'Delete snapshot?', confirmText: 'Delete', danger: true }))) return;
    run('Snapshot deleted', () => api.del(`/api/games/${game.id}/snapshot/${snap.id}`));
  }

  async function browseSnapshot(snap) {
    try {
      const files = await api.get(`/api/games/${game.id}/snapshot/${snap.id}/files`);
      browsing = { snap, files };
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  const restoreFile = async (relPath) => {
    if (!(await askConfirm(`Put "${relPath}" back from ${named(browsing.snap)}, over the file there now? The current file is kept in a snapshot first.`, { title: 'Restore file?', confirmText: 'Restore' }))) return;
    return run(`Restored ${relPath}`, () =>
      api.post(`/api/games/${game.id}/snapshot/${browsing.snap.id}/restore-file`, { relPath })
    );
  };
</script>

<div class="card snap-new">
  <input placeholder="Snapshot comment (optional)" bind:value={comment} on:keydown={(e) => e.key === 'Enter' && takeSnapshot()} />
  <button class="btn primary" disabled={$busy} on:click={takeSnapshot}><Camera size={16} />Snapshot now</button>
</div>

{#if browsing}
  <div class="card browse">
    <div class="browse-head">
      <h3>Files in {named(browsing.snap)}</h3>
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
  <div bind:this={listEl}>
  {#each groups as group (group.day)}
    <h4 class="day">{group.day}</h4>
    <div class="list">
      {#each group.snaps as snap (snap.id)}
        {@const k = snapshotKind(snap, now)}
        <div class="row kind-{k.kind}" class:open={browsing?.snap.id === snap.id} class:flash={flashing === snap.id} data-snap={snap.id}>
          <div class="when" title={new Date(snap.timestamp).toLocaleString()}>
            <span class="time">{new Date(snap.timestamp).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}</span>
            <span class="ago">{timeAgo(snap.timestamp, now.getTime())}</span>
          </div>
          <div class="info">
            <div class="title">
              <span class="title-text">{k.title}</span>
              {#if snap.id === latestId}<span class="tag latest">Latest</span>{/if}
            </div>
            {#if noting === snap.id}
              <input
                id="note-{snap.id}"
                class="note-input"
                placeholder="A note about this snapshot"
                maxlength="500"
                bind:value={noteDraft}
                on:keydown={(e) => noteKeydown(e, snap)}
                on:blur={() => saveNote(snap)}
              />
            {:else if snap.note}
              <button class="note" title="Edit the note" on:click={() => startNote(snap)}>{snap.note}</button>
            {/if}
            <div class="meta">
              {#if snap.pinned}<span class="tag pinned"><Pin size={10} strokeWidth={2.6} />Pinned</span>{/if}
              <span class="tag kind">{k.label}</span>
              {#if branchCount > 1}<span class="tag">{snap.branch}</span>{/if}
              <span>{fmtSize(snap.sizeBytes)}</span>
            </div>
          </div>
          <div class="actions">
            <button
              class="btn small ghost icon"
              class:on={snap.pinned}
              disabled={$busy}
              title={snap.pinned ? 'Unpin: let the snapshot limits apply to it again' : 'Pin: keep it through every limit and clean-up'}
              aria-label={snap.pinned ? 'Unpin' : 'Pin'}
              aria-pressed={snap.pinned}
              on:click={() => togglePin(snap)}
            >
              <svelte:component this={snap.pinned ? PinOff : Pin} size={14} />
            </button>
            <button class="btn small ghost icon" title={snap.note ? 'Edit the note' : 'Add a note'} aria-label="Note" on:click={() => startNote(snap)}>
              <Pencil size={13} />
            </button>
            <button class="btn small" on:click={() => browseSnapshot(snap)}>Files</button>
            <button class="btn small" disabled={$busy} on:click={() => rollback(snap)}>Restore</button>
            <button class="btn small ghost-danger" disabled={$busy} title="Delete this snapshot" on:click={() => deleteSnapshot(snap)}>Delete</button>
          </div>
        </div>
      {/each}
    </div>
  {/each}
  </div>
{/if}

<style>
  .snap-new {
    display: flex;
    gap: 10px;
    padding: 14px;
    margin-bottom: 18px;
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
  .day {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-faint);
    margin: 18px 0 8px;
  }
  .day:first-of-type {
    margin-top: 0;
  }
  .list {
    display: flex;
    flex-direction: column;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }
  /* Opened on this one (from Ctrl+K): marked for a moment. */
  .row.flash {
    box-shadow: inset 0 0 0 2px var(--accent);
    background: var(--accent-soft);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 11px 16px;
    border-top: 1px solid var(--border);
    border-left: 3px solid transparent;
  }
  .row:first-child {
    border-top: none;
  }
  .row:hover,
  .row.open {
    background: var(--bg-hover);
  }
  /* A copy kept before OpenSave replaced the save is the one you look for
     after something went wrong, so it is marked out from the rest. */
  .row.kind-safety {
    border-left-color: var(--warn);
  }
  .row.kind-manual {
    border-left-color: var(--accent);
  }
  /* The save as a play session left it: a save point by nature. */
  .row.kind-session {
    border-left-color: var(--success);
  }
  .when {
    display: flex;
    flex-direction: column;
    width: 78px;
    flex-shrink: 0;
  }
  .time {
    font-weight: 600;
    font-size: 0.9rem;
    font-variant-numeric: tabular-nums;
  }
  .ago {
    font-size: 0.72rem;
    color: var(--text-faint);
  }
  .info {
    flex: 1;
    min-width: 0;
  }
  .title {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.88rem;
    margin-bottom: 3px;
  }
  .title-text {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .meta {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.74rem;
    color: var(--text-faint);
  }
  .tag {
    border: 1px solid var(--border-strong);
    border-radius: 999px;
    padding: 0 7px;
    font-size: 0.68rem;
    line-height: 1.6;
    color: var(--text-dim);
    white-space: nowrap;
  }
  .kind-safety .tag.kind {
    border-color: rgba(var(--warn-rgb), 0.45);
    color: var(--warn);
  }
  .kind-manual .tag.kind {
    border-color: var(--accent);
    color: var(--accent);
  }
  .kind-session .tag.kind {
    border-color: rgba(var(--success-rgb), 0.45);
    color: var(--success);
  }
  .tag.latest {
    border-color: rgba(var(--success-rgb), 0.45);
    color: var(--success);
  }
  .tag.pinned {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    border-color: transparent;
    background: var(--accent-soft);
    color: var(--accent);
  }
  .btn.icon.on {
    color: var(--accent);
  }
  .note {
    display: block;
    max-width: 100%;
    margin: 1px 0 3px;
    padding: 0;
    border: none;
    background: none;
    color: var(--text-dim);
    font: inherit;
    font-size: 0.82rem;
    font-style: italic;
    text-align: left;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    cursor: text;
  }
  .note:hover {
    color: var(--text);
  }
  .note-input {
    width: 100%;
    margin: 3px 0 4px;
    padding: 4px 8px;
    background: var(--bg);
    border: 1px solid var(--accent);
    border-radius: 7px;
    color: var(--text);
    font: inherit;
    font-size: 0.84rem;
    outline: none;
  }
  .actions {
    display: flex;
    gap: 6px;
    flex-shrink: 0;
  }
  /* Delete is available, not suggested: quiet until pointed at. */
  .ghost-danger {
    background: transparent;
    border-color: transparent;
    color: var(--text-faint);
  }
  .ghost-danger:hover:not(:disabled) {
    background: rgba(var(--danger-rgb), 0.16);
    border-color: rgba(var(--danger-rgb), 0.4);
    color: var(--danger-text);
  }
  .browse {
    margin-bottom: 18px;
  }
  .browse-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 10px;
  }
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
