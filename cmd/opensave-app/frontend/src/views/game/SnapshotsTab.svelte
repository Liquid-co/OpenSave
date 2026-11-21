<script>
  // History: every snapshot of this game, newest first and grouped by day.
  // Each row has one action, Restore; the rest (its files, comparing it,
  // pinning, a note, deleting) is in its ⋯ menu. The coloured edge says why
  // it was taken, as everywhere snapshots are listed.
  //
  // Taking one: Take snapshot (above the tabs, or the page's ⋯ menu) opens a
  // line at the top to name it; Enter takes it, named or not.
  //
  // Motion: the rows arrive one after another, moving only; a snapshot just
  // taken lights up as it lands; one opened from elsewhere is marked a moment.
  import { onDestroy, tick as rendered } from 'svelte';
  import { askConfirm, toast } from '../../lib/stores.js';
  import { api, native } from '../../lib/api.js';
  import { restoreKey } from '../../lib/toasts.js';
  import { fmtSize } from '../../lib/format.js';
  import { groupByDay, snapshotKind, whenLabel } from '../../lib/snapshots.js';
  import { askRestore } from '../../lib/restore.js';
  import { t } from '../../lib/i18n.js';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { collapse } from '../../lib/motion.js';
  import Camera from 'lucide-svelte/icons/camera';
  import Pin from 'lucide-svelte/icons/pin';
  import PinOff from 'lucide-svelte/icons/pin-off';
  import Pencil from 'lucide-svelte/icons/pencil';
  import GitCompareArrows from 'lucide-svelte/icons/git-compare-arrows';
  import FileText from 'lucide-svelte/icons/file-text';
  import Trash2 from 'lucide-svelte/icons/trash-2';
  import MoreHorizontal from 'lucide-svelte/icons/ellipsis';
  import History from 'lucide-svelte/icons/history';
  import CompareDialog from './CompareDialog.svelte';
  import SnapshotFiles from './SnapshotFiles.svelte';
  import Dropdown from '../../components/ui/Dropdown.svelte';

  export let game;
  export let runner;
  /** A snapshot to bring into view and mark, when the page was opened on it. */
  export let focus = null;
  /** Changes when Take snapshot was asked for elsewhere on the page. */
  export let takeRequest = 0;
  const { busy, run } = runner;

  // snapshotKind's fixed titles and tag, said in the reader's language; a
  // daemon comment has no translation and passes through as text.
  const kindSay = (k) => {
    if (k.kind === 'manual')
      return { title: k.title === 'Snapshot' ? { key: 'snapshots.kind.snapshot' } : { text: k.title }, label: { key: 'activity.event.takenByYou' } };
    if (k.kind === 'auto') return { title: { key: 'activity.event.saveChanged' }, label: { key: 'snapshots.kind.automatic' } };
    if (k.kind === 'start') return { title: { key: 'activity.event.trackingStart' }, label: { key: 'snapshots.kind.automatic' } };
    if (k.kind === 'session') return { title: { text: k.title }, label: { key: 'backups.kind.session' } };
    if (k.kind === 'safety') return { title: { text: k.title }, label: { key: 'backups.kind.safety' } };
    return { title: { text: k.title }, label: { key: 'snapshots.kind.automatic' } };
  };
  const say = (x) => x.text ?? $t(x.key, x.params);
  const agoKey = (at, now = Date.now()) => {
    const p = timeAgoParts(at, now);
    if (p.kind === 'invalid') return p.text;
    const { kind, ...params } = p;
    return $t('timeAgo.' + kind, params);
  };

  // ── Opened on one ──
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

  let now = new Date();
  const tick = setInterval(() => (now = new Date()), 30_000);
  onDestroy(() => clearInterval(tick));

  $: branchCount = Object.keys(game.branches ?? {}).length;
  $: allSnapshots = Object.values(game.branches ?? {})
    .flatMap((b) => (b.snapshots ?? []).map((s) => ({ ...s, branch: b.name })))
    .sort((a, b) => (a.timestamp < b.timestamp ? 1 : -1));
  $: groups = groupByDay(allSnapshots, now);
  $: latestId = allSnapshots.find((s) => s.branch === game.activeBranch)?.id;

  // A snapshot that appears while the page is open is one just taken (or
  // just arrived): it lights up as it lands.
  let known = null;
  let fresh = new Set();
  $: noteFresh(allSnapshots);
  function noteFresh(list) {
    const ids = list.map((s) => s.id);
    if (known) {
      const added = ids.filter((id) => !known.has(id));
      if (added.length) {
        fresh = new Set([...fresh, ...added]);
        setTimeout(() => {
          fresh = new Set([...fresh].filter((id) => !added.includes(id)));
        }, 1600);
      }
    }
    known = new Set(ids);
  }

  const named = (snap) => $t('game.snapshots.named', { when: whenLabel(snap.timestamp, now) });

  // ── Taking one ──
  let composing = false;
  let comment = '';
  let composer;
  let lastRequest = takeRequest;
  $: if (takeRequest !== lastRequest) {
    lastRequest = takeRequest;
    compose();
  }
  async function compose() {
    composing = true;
    await rendered();
    composer?.focus();
  }
  const takeSnapshot = () =>
    run($t('game.snapshots.created'), async () => {
      await api.post(`/api/games/${game.id}/snapshot`, { comment });
      comment = '';
      composing = false;
    });
  function composerKeys(e) {
    if (e.key === 'Enter') takeSnapshot();
    else if (e.key === 'Escape') {
      e.stopPropagation();
      composing = false;
    }
  }

  // ── Acting on one ──
  const rollback = async (snap) => {
    if (!(await askRestore(game, snap))) return;
    return run($t('game.snapshots.restored'), () => api.post(`/api/games/${game.id}/rollback`, { snapshotId: snap.id }), {
      key: restoreKey(game.id),
      action: { label: $t('game.header.openFolder'), run: () => native.openFolder(game.savePath) }
    });
  };
  // Pinned: no limit, age rule or clean-up removes it. Not a lock.
  const togglePin = (snap) =>
    run(snap.pinned ? $t('game.snapshots.unpinned') : $t('game.snapshots.pinned'), () =>
      api.patch(`/api/games/${game.id}/snapshot/${snap.id}`, { pinned: !snap.pinned })
    );
  // The note is edited in place; Enter or leaving the field saves it,
  // Escape puts it back.
  let noting = null;
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
    const pinnedLine = snap.pinned ? $t('game.snapshots.deletePinned') : '';
    if (!(await askConfirm($t('game.snapshots.deleteConfirm', { snap: named(snap), pinned: pinnedLine }), { title: $t('game.snapshots.deleteTitle'), confirmText: $t('common.delete'), danger: true }))) return;
    run($t('game.snapshots.deleted'), () => api.del(`/api/games/${game.id}/snapshot/${snap.id}`));
  }

  let filesOf = null; // the snapshot whose files are open
  let comparing = null;
  let menuFor = null; // the row whose ⋯ menu is open
  const menuOf = (snap) => [
    { label: $t('game.snapshots.files'), icon: FileText, run: () => (filesOf = snap) },
    ...(allSnapshots.length > 1 ? [{ label: $t('game.snapshots.compare'), icon: GitCompareArrows, run: () => (comparing = snap) }] : []),
    { label: snap.pinned ? $t('game.snapshots.unpin') : $t('game.snapshots.pin'), icon: snap.pinned ? PinOff : Pin, hint: snap.pinned ? '' : $t('game.history.pinHint'), run: () => togglePin(snap) },
    { label: snap.note ? $t('game.snapshots.editNote') : $t('game.snapshots.addNote'), icon: Pencil, run: () => startNote(snap) },
    null,
    { label: $t('common.delete'), icon: Trash2, danger: true, run: () => deleteSnapshot(snap) }
  ];
</script>

{#if composing}
  <div class="composer" transition:collapse>
    <span class="ctile"><Camera size={17} /></span>
    <input bind:this={composer} placeholder={$t('game.snapshots.commentPlaceholder')} bind:value={comment} on:keydown={composerKeys} maxlength="200" />
    <button class="btn primary" disabled={$busy} on:click={takeSnapshot}>{$t('game.snapshots.takeNow')}</button>
    <button class="btn ghost" on:click={() => (composing = false)}>{$t('common.cancel')}</button>
  </div>
{/if}

{#if filesOf}
  <SnapshotFiles {game} {runner} snap={filesOf} on:close={() => (filesOf = null)} />
{/if}
{#if comparing}
  <CompareDialog {game} snapshots={allSnapshots} start={comparing} onClose={() => (comparing = null)} />
{/if}

{#if allSnapshots.length === 0}
  <div class="empty">
    <span class="empty-icon"><History size={24} /></span>
    <h3>{$t('game.snapshots.emptyTitle')}</h3>
    <p>{$t('game.snapshots.emptyBody')}</p>
  </div>
{:else}
  <div bind:this={listEl} class="days">
    {#each groups as group, gi (group.day)}
      <section class="group" style="--g: {Math.min(gi, 6)}">
        <h4 class="day">{group.day === 'Today' ? $t('snapshots.day.today') : group.day === 'Yesterday' ? $t('snapshots.day.yesterday') : group.day}</h4>
        <div class="list">
          {#each group.snaps as snap, si (snap.id)}
            {@const k = snapshotKind(snap, now)}
            {@const s = kindSay(k)}
            <div class="row kind-{k.kind}" class:flash={flashing === snap.id} class:fresh={fresh.has(snap.id)} data-snap={snap.id} style="--i: {Math.min(si, 10)}">
              <div class="when" title={new Date(snap.timestamp).toLocaleString()}>
                <span class="time">{new Date(snap.timestamp).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}</span>
                <span class="ago">{agoKey(snap.timestamp, now.getTime())}</span>
              </div>
              <div class="info">
                <div class="title">
                  <span class="title-text">{say(s.title)}</span>
                  {#if snap.id === latestId}<span class="tag latest">{$t('game.snapshots.latest')}</span>{/if}
                  {#if snap.problem}<span class="tag damaged" title={snap.problem}>{$t('game.snapshots.damaged')}</span>{/if}
                  {#if snap.pinned}<span class="tag pinned"><Pin size={10} strokeWidth={2.6} />{$t('game.snapshots.pinnedTag')}</span>{/if}
                </div>
                {#if noting === snap.id}
                  <input
                    id="note-{snap.id}"
                    class="note-input"
                    placeholder={$t('game.snapshots.notePlaceholder')}
                    maxlength="500"
                    bind:value={noteDraft}
                    on:keydown={(e) => noteKeydown(e, snap)}
                    on:blur={() => saveNote(snap)}
                  />
                {:else if snap.note}
                  <button class="note" title={$t('game.snapshots.editNote')} on:click={() => startNote(snap)}>{snap.note}</button>
                {/if}
                <div class="meta">
                  <span class="tag kind">{say(s.label)}</span>
                  {#if branchCount > 1}<span class="tag">{snap.branch}</span>{/if}
                  <span>{fmtSize(snap.sizeBytes)}</span>
                </div>
              </div>
              <div class="actions">
                <button
                  class="btn small restore"
                  disabled={$busy || !!snap.problem}
                  title={snap.problem ? $t('game.snapshots.cannotRestore', { problem: snap.problem }) : undefined}
                  on:click={() => rollback(snap)}>{$t('game.snapshots.restore')}</button
                >
                <span class="more-wrap">
                  <button
                    class="more"
                    class:on={menuFor === snap.id}
                    aria-label={$t('game.history.more', { snap: named(snap) })}
                    aria-haspopup="menu"
                    aria-expanded={menuFor === snap.id}
                    on:click={() => (menuFor = menuFor === snap.id ? null : snap.id)}><MoreHorizontal size={17} /></button
                  >
                  {#if menuFor === snap.id}
                    <Dropdown items={menuOf(snap)} open={true} label={$t('game.history.more', { snap: named(snap) })} on:close={() => (menuFor = null)} />
                  {/if}
                </span>
              </div>
            </div>
          {/each}
        </div>
      </section>
    {/each}
  </div>
{/if}

<style>
  .composer {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 16px;
    padding: 10px 10px 10px 12px;
    border-radius: var(--radius-lg);
    border: 1px solid rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.5);
    background: var(--bg-raised);
  }
  .ctile {
    width: 34px;
    height: 34px;
    flex: none;
    border-radius: 10px;
    display: grid;
    place-items: center;
    background: rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.18);
    color: var(--game-text, var(--accent-text));
  }
  .composer input {
    flex: 1;
    min-width: 0;
  }
  .days {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  .day {
    margin: 0 0 8px 2px;
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-faint);
    animation: arrive 0.32s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--g) * 50ms);
  }
  .list {
    display: flex;
    flex-direction: column;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }
  .row {
    position: relative;
    display: flex;
    align-items: center;
    gap: 16px;
    min-height: 56px;
    padding: 8px 12px 8px 16px;
    border-top: 1px solid var(--border);
    box-shadow: inset 3px 0 0 transparent;
    animation: arrive 0.32s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--g) * 50ms + var(--i) * 25ms);
    transition:
      background 0.15s,
      box-shadow 0.15s;
  }
  .row:first-child {
    border-top: none;
    border-radius: var(--radius-lg) var(--radius-lg) 0 0;
  }
  .row:last-child {
    border-radius: 0 0 var(--radius-lg) var(--radius-lg);
  }
  .row:only-child {
    border-radius: var(--radius-lg);
  }
  .row:hover {
    background: var(--bg-hover);
  }
  /* Why it was taken, on its edge: by you accent, after playing green, a
     safety copy amber. Plain automatic ones have none. */
  .row.kind-manual {
    box-shadow: inset 3px 0 0 var(--accent);
  }
  .row.kind-session {
    box-shadow: inset 3px 0 0 var(--success);
  }
  .row.kind-safety {
    box-shadow: inset 3px 0 0 var(--warn);
  }
  /* Opened on this one (from Ctrl+K or the timeline): marked a moment. */
  .row.flash {
    background: rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.14);
  }
  /* Just taken: it lights up as it lands, then settles. */
  .row.fresh {
    animation: landed 1.6s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  @keyframes landed {
    0% {
      transform: translateY(-6px);
      background: rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.22);
    }
    30% {
      transform: none;
    }
    100% {
      background: transparent;
    }
  }
  .when {
    display: flex;
    flex-direction: column;
    width: 82px;
    flex-shrink: 0;
  }
  .time {
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .ago {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .title {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .title-text {
    font-weight: 600;
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
    flex: none;
    border: 1px solid var(--border-strong);
    border-radius: 999px;
    padding: 0 7px;
    font-size: var(--text-xs);
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
    color: var(--accent-text);
  }
  .kind-session .tag.kind {
    border-color: rgba(var(--success-rgb), 0.45);
    color: var(--success);
  }
  .tag.damaged {
    border-color: rgba(var(--danger-rgb), 0.5);
    color: var(--danger-text);
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
    color: var(--accent-text);
  }
  .note {
    display: block;
    max-width: 100%;
    padding: 0;
    border: none;
    background: none;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-md);
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
    margin: 2px 0;
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }
  /* Restore shows on the row pointed at, or the one with focus inside;
     always there to a keyboard or a touch screen. */
  .restore {
    opacity: 0.0001;
    transition: opacity 0.15s;
  }
  .row:hover .restore,
  .row:focus-within .restore,
  .row.flash .restore {
    opacity: 1;
  }
  @media (hover: none) {
    .restore {
      opacity: 1;
    }
  }
  .more-wrap {
    position: relative;
  }
  .more {
    width: 32px;
    height: 32px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
    transition:
      background 0.15s,
      color 0.15s;
  }
  .more:hover,
  .more.on {
    background: var(--bg-active);
    color: var(--text);
  }
  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: 36px 20px;
    border-radius: var(--radius-lg);
    border: 1.5px dashed var(--border-strong);
    text-align: center;
    color: var(--text-dim);
  }
  .empty-icon {
    width: 48px;
    height: 48px;
    border-radius: 14px;
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--text-faint);
  }
</style>
