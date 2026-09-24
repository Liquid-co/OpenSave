<script>
  // A disagreement about one of a game's EXTRA save folders.
  //
  // Deliberately its own screen rather than a mode of the main conflict
  // modal. That one offers "Keep both", which parks the peer's copy on a
  // branch — and branches belong to the game as a whole, so the same button
  // here would do something to the save folder while the user was looking at
  // a settings folder. Two answers only, and the screen says which folder it
  // is asking about in every sentence.
  import { locationConflicts, games, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import { demandAttention } from '../lib/notify.js';
  import Chevron from './ui/Chevron.svelte';
  import Laptop from 'lucide-svelte/icons/laptop';
  import Monitor from 'lucide-svelte/icons/monitor';
  import ShieldCheck from 'lucide-svelte/icons/shield-check';
  import FolderOpen from 'lucide-svelte/icons/folder-open';

  let busy = false;
  let showDiff = false;
  let applying = new Set(); // "gameId\u0000root" of resolutions in flight
  let seen = new Set();

  const keyOf = (c) => `${c.gameId}\u0000${c.root}`;

  $: pending = ($locationConflicts ?? []).filter((c) => !applying.has(keyOf(c)));
  $: current = pending[0]; // one at a time, like the whole-game screen
  $: gameName = current ? ($games[current.gameId]?.name ?? current.gameId) : '';
  $: peerName = current ? (current.peer?.name ?? current.peer?.Name ?? 'the other device') : '';

  // Announce new ones the same way a save conflict is announced: this is
  // still "something of yours needs a decision before it can sync".
  $: announce($locationConflicts ?? []);
  function announce(list) {
    const fresh = list.filter((c) => !seen.has(keyOf(c)));
    if (fresh.length > 0) {
      demandAttention();
      const name = $games[fresh[0].gameId]?.name ?? 'a game';
      toast(`The “${fresh[0].root}” folder of “${name}” differs on both devices`, 'error');
    }
    seen = new Set(list.map(keyOf));
    // Forget in-flight markers for conflicts that are gone.
    const live = new Set(list.map(keyOf));
    let pruned = false;
    for (const k of applying) {
      if (!live.has(k)) {
        applying.delete(k);
        pruned = true;
      }
    }
    if (pruned) applying = new Set(applying);
  }

  async function resolve(resolution) {
    if (!current || busy) return;
    busy = true;
    const c = current;
    try {
      await api.post(`/api/games/${c.gameId}/resolve-location-conflict`, {
        peerId: c.peer?.id ?? c.peer?.ID,
        root: c.root,
        resolution
      });
      applying = new Set(applying).add(keyOf(c));
      if (resolution === 'keep-remote') {
        toast(`Bringing ${peerName}'s “${c.root}” folder over…`, 'info');
      }
      showDiff = false;
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  const fmtMs = (ms) => (ms ? new Date(ms).toLocaleString() : 'unknown');
  const fmtSize = (n) =>
    n < 0 ? '—' : n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : n >= 1024 ? (n / 1024).toFixed(1) + ' KB' : n + ' B';
  const diffIcon = (s) => (s === 'changed' ? '✱' : s === 'only-remote' ? '+' : '−');
  const diffLabel = (s) =>
    s === 'changed' ? 'differs' : s === 'only-remote' ? `only on ${peerName}` : 'only on this device';

  $: localMs = current?.localStats?.latestMtimeMs ?? 0;
  $: remoteMs = current?.remoteStats?.latestMtimeMs ?? 0;
  $: newerSide = localMs && remoteMs ? (localMs > remoteMs ? 'local' : remoteMs > localMs ? 'remote' : '') : '';

  $: diffFiles = current?.diffFiles ?? [];
  $: changedCount = diffFiles.filter((d) => d.status === 'changed').length;
  $: onlyLocalCount = diffFiles.filter((d) => d.status === 'only-local').length;
  $: onlyRemoteCount = diffFiles.filter((d) => d.status === 'only-remote').length;

  function sideSummary(stats) {
    const files = stats?.files;
    if (files === 0) return 'folder is empty';
    if (typeof files !== 'number') return '';
    return `whole folder: ${files} file${files === 1 ? '' : 's'} · ${fmtSize(stats?.totalBytes ?? -1)}`;
  }
</script>

{#if current}
  <div class="overlay">
    <div class="modal card">
      <h3 class="with-icon"><FolderOpen size={20} /> “{current.root}” folder — {gameName}</h3>
      <p class="desc">
        This is one of {gameName}'s extra save folders, and it changed on both this device and
        <strong>{peerName}</strong>. The game's main save is unaffected and keeps syncing; only this
        folder is waiting on you.
      </p>

      <div class="versions">
        <div class="version" class:newer={newerSide === 'local'}>
          <div class="v-head">
            <span class="v-title with-icon"><Laptop size={16} /> This device</span>
            {#if newerSide === 'local'}<span class="v-badge">changed more recently</span>{/if}
          </div>
          <div class="v-diff">
            <strong>{changedCount + onlyLocalCount}</strong>
            file{changedCount + onlyLocalCount === 1 ? '' : 's'} differ{changedCount + onlyLocalCount === 1 ? 's' : ''} here
          </div>
          {#if onlyLocalCount > 0}
            <div class="v-only">{onlyLocalCount} only on this device</div>
          {/if}
          <div class="v-stats">{sideSummary(current.localStats)}</div>
          <div class="v-time">last change {fmtMs(localMs)}</div>
        </div>
        <div class="version" class:newer={newerSide === 'remote'}>
          <div class="v-head">
            <span class="v-title with-icon"><Monitor size={16} /> {peerName}</span>
            {#if newerSide === 'remote'}<span class="v-badge">changed more recently</span>{/if}
          </div>
          <div class="v-diff">
            <strong>{changedCount + onlyRemoteCount}</strong>
            file{changedCount + onlyRemoteCount === 1 ? '' : 's'} differ{changedCount + onlyRemoteCount === 1 ? 's' : ''} there
          </div>
          {#if onlyRemoteCount > 0}
            <div class="v-only">{onlyRemoteCount} only on {peerName}</div>
          {/if}
          <div class="v-stats">{sideSummary(current.remoteStats)}</div>
          <div class="v-time">last change {fmtMs(remoteMs)}</div>
        </div>
      </div>

      {#if current.diffTotal > 0}
        <button class="diff-toggle" on:click={() => (showDiff = !showDiff)}>
          <Chevron open={showDiff} /> What's different ({current.diffTotal} file{current.diffTotal === 1 ? '' : 's'})
        </button>
        {#if showDiff}
          <div class="diff-list">
            {#each diffFiles as d (d.path)}
              <div class="diff-row">
                <span class="diff-icon" data-status={d.status}>{diffIcon(d.status)}</span>
                <span class="diff-path" title={d.path}>{d.path}</span>
                <span class="diff-meta">{diffLabel(d.status)}</span>
              </div>
            {/each}
            {#if current.diffTotal > diffFiles.length}
              <div class="diff-more">…and {current.diffTotal - diffFiles.length} more</div>
            {/if}
          </div>
        {/if}
      {/if}

      <div class="actions">
        <button class="btn" disabled={busy} on:click={() => resolve('keep-remote')}>
          Use {peerName}'s
        </button>
        <button class="btn primary" disabled={busy} on:click={() => resolve('keep-local')}>
          Keep mine
        </button>
      </div>
      <p class="hint-line">
        <ShieldCheck size={15} class="inline-icon" /> A snapshot of every one of this game's folders is taken before either choice is applied,
        so both are undoable from the Snapshots tab. <strong>“Keep mine”</strong> makes this
        device's copy the shared one and asks {peerName} to take it — {peerName} gets its own say if
        it has newer work. <strong>“Use {peerName}'s”</strong> brings their copy here.
      </p>
      <p class="hint-line">
        There is no “keep both” for a folder: that option puts the other copy on a branch, and
        branches cover the whole game rather than one of its folders.
      </p>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    /* Above the whole-game conflict screen: if both are open, the save
       folder is the more urgent of the two and should be answered first, so
       it must NOT be buried. This sits below it. */
    z-index: 88;
    padding: 32px;
  }
  .modal {
    width: 580px;
    max-width: 100%;
    max-height: calc(100vh - 80px);
    overflow-y: auto;
  }
  h3 {
    margin-bottom: 8px;
    overflow-wrap: anywhere;
  }
  .desc {
    color: var(--text-dim);
    font-size: 0.9rem;
    margin-bottom: 16px;
  }
  .versions {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
    margin-bottom: 14px;
  }
  .version {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 12px;
  }
  .version.newer {
    border-color: rgba(var(--success-rgb), 0.45);
  }
  .v-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
    margin-bottom: 8px;
  }
  .v-title {
    font-weight: 600;
    font-size: 0.9rem;
  }
  .v-badge {
    font-size: 0.66rem;
    font-weight: 700;
    color: var(--success);
    background: rgba(var(--success-rgb), 0.12);
    padding: 2px 7px;
    border-radius: 999px;
    white-space: nowrap;
  }
  .v-diff {
    font-size: 0.84rem;
    color: var(--text);
    margin-bottom: 3px;
  }
  .v-diff strong {
    font-weight: 700;
    color: var(--accent);
  }
  .v-only {
    font-size: 0.76rem;
    color: var(--text-dim);
    margin-bottom: 3px;
  }
  .v-stats {
    font-size: 0.78rem;
    color: var(--text-faint);
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--border);
  }
  .v-time {
    font-size: 0.74rem;
    color: var(--text-faint);
  }
  .diff-toggle {
    border: none;
    background: transparent;
    color: var(--text-dim);
    font-size: 0.84rem;
    font-weight: 600;
    cursor: pointer;
    padding: 4px 0;
    margin-bottom: 6px;
  }
  .diff-toggle:hover {
    color: var(--text);
  }
  .diff-list {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 8px 12px;
    margin-bottom: 14px;
    max-height: 180px;
    overflow-y: auto;
  }
  .diff-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 0;
    font-size: 0.8rem;
  }
  .diff-icon {
    width: 16px;
    text-align: center;
    font-weight: 700;
    flex-shrink: 0;
  }
  .diff-icon[data-status='changed'] {
    color: var(--warn);
  }
  .diff-icon[data-status='only-remote'] {
    color: var(--success);
  }
  .diff-icon[data-status='only-local'] {
    color: var(--danger);
  }
  .diff-path {
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
    font-size: 0.76rem;
  }
  .diff-meta {
    color: var(--text-faint);
    font-size: 0.72rem;
    white-space: nowrap;
  }
  .diff-more {
    color: var(--text-faint);
    font-size: 0.76rem;
    padding: 6px 0 2px;
    text-align: center;
  }
  .actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }
  .hint-line {
    margin-top: 12px;
    font-size: 0.78rem;
    color: var(--text-faint);
    line-height: 1.5;
  }
</style>
