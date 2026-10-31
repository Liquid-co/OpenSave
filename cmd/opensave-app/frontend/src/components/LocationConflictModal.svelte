<script>
  // A disagreement about one of a game's EXTRA save folders.
  //
  // Deliberately its own screen rather than a mode of the main conflict
  // modal. That one offers "Keep both", which parks the peer's copy on a
  // branch — and branches belong to the game as a whole, so the same button
  // here would do something to the save folder while the user was looking at
  // a settings folder. Two answers only, and the screen says which folder it
  // is asking about in every sentence.
  import { dialogOut } from '../lib/motion.js';
  import { locationConflicts, games, peers, settings, toast, view } from '../lib/stores.js';
  import { isHandheld } from '../lib/devices.js';
  import { visited } from '../lib/later.js';
  import { api } from '../lib/api.js';
  import { demandAttention } from '../lib/notify.js';
  import { t, segments } from '../lib/i18n.js';
  import Chevron from './ui/Chevron.svelte';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Monitor from 'lucide-svelte/icons/monitor';
  import History from 'lucide-svelte/icons/history';
  import FolderOpen from 'lucide-svelte/icons/folder-open';

  let busy = false;
  let showDiff = false;
  let applying = new Set(); // "gameId\u0000root" of resolutions in flight
  let seen = new Set();

  const keyOf = (c) => `${c.gameId}\u0000${c.root}`;

  // Put off as on the whole-game screen (ConflictModal): out of the way until
  // the game's page is opened again.
  let later = new Map(); // keyOf(c) → the view it was put off from
  $: later = visited(later, $view, (key) => key.split('\u0000')[0]);
  const putOff = () => {
    if (!current || busy) return;
    later = new Map(later).set(keyOf(current), $view);
    showDiff = false;
  };
  const onKey = (e) => {
    if (e.key === 'Escape' && current) putOff();
  };

  $: pending = ($locationConflicts ?? []).filter((c) => !applying.has(keyOf(c)) && !later.has(keyOf(c)));
  $: current = pending[0]; // one at a time, like the whole-game screen
  $: gameName = current ? ($games[current.gameId]?.name ?? current.gameId) : '';
  $: peerName = current ? (current.peer?.name ?? current.peer?.Name ?? $t('conflict.otherDevice')) : '';
  $: peerType = current ? $peers[current.peer?.id ?? current.peer?.ID]?.deviceType : '';
  $: localIcon = isHandheld($settings?.deviceType) ? Gamepad2 : Monitor;
  $: peerIcon = isHandheld(peerType) ? Gamepad2 : Monitor;

  // Announce new ones the same way a save conflict is announced: this is
  // still "something of yours needs a decision before it can sync".
  $: announce($locationConflicts ?? []);
  function announce(list) {
    const fresh = list.filter((c) => !seen.has(keyOf(c)));
    if (fresh.length > 0) {
      const game = $games[fresh[0].gameId];
      const name = game?.name ?? $t('conflict.aGame');
      demandAttention('conflicts', { title: name, body: $t('locConflict.attentionBody', { root: fresh[0].root }), game });
      toast($t('locConflict.attentionToast', { root: fresh[0].root, name }), 'error');
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
        toast($t('locConflict.bringingToast', { peer: peerName, root: c.root }), 'info');
      }
      showDiff = false;
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  const fmtMs = (ms) => (ms ? new Date(ms).toLocaleString() : $t('conflict.unknown'));
  const fmtSize = (n) =>
    n < 0 ? '—' : n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : n >= 1024 ? (n / 1024).toFixed(1) + ' KB' : n + ' B';
  const diffIcon = (s) => (s === 'changed' ? '✱' : s === 'only-remote' ? '+' : '−');
  const diffLabel = (s) =>
    s === 'changed'
      ? $t('conflict.differs')
      : s === 'only-remote'
        ? $t('conflict.onlyOnPeer', { peer: peerName })
        : $t('conflict.onlyOnDevice');

  // The hint paragraph reads as one sentence stream in every language; the
  // option names arrive already quoted and are bolded in place.
  const hintUndoParams = (peerName) => ({
    mine: $t('conflict.keepMineQuoted'),
    theirs: $t('locConflict.useTheirsQuoted', { peer: peerName }),
    peer: peerName
  });
  $: hintUndoParts = segments(
    $t('locConflict.hintUndo', hintUndoParams(peerName)),
    hintUndoParams(peerName),
    ['mine', 'theirs']
  );

  $: localMs = current?.localStats?.latestMtimeMs ?? 0;
  $: remoteMs = current?.remoteStats?.latestMtimeMs ?? 0;
  $: newerSide = localMs && remoteMs ? (localMs > remoteMs ? 'local' : remoteMs > localMs ? 'remote' : '') : '';

  $: diffFiles = current?.diffFiles ?? [];
  $: changedCount = diffFiles.filter((d) => d.status === 'changed').length;
  $: onlyLocalCount = diffFiles.filter((d) => d.status === 'only-local').length;
  $: onlyRemoteCount = diffFiles.filter((d) => d.status === 'only-remote').length;

  function sideSummary(stats) {
    const files = stats?.files;
    if (files === 0) return $t('locConflict.folderEmpty');
    if (typeof files !== 'number') return '';
    return $t('locConflict.wholeFolder', { n: files, size: fmtSize(stats?.totalBytes ?? -1) });
  }
</script>

<svelte:window on:keydown={onKey} />

{#if current}
  <div class="overlay" out:dialogOut|global>
    <div class="modal card">
      <h3 class="with-icon"><FolderOpen size={20} /> {$t('locConflict.title', { root: current.root, game: gameName })}</h3>
      <p class="desc">
        {#each segments($t('locConflict.desc', { game: gameName, peer: peerName }), { game: gameName, peer: peerName }, ['peer']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
      </p>

      <div class="versions">
        <div class="version" class:newer={newerSide === 'local'}>
          <div class="v-head">
            <span class="v-title with-icon"><svelte:component this={localIcon} size={16} /> {$t('conflict.thisDevice')}</span>
            {#if newerSide === 'local'}<span class="v-badge">{$t('locConflict.changedRecently')}</span>{/if}
          </div>
          <div class="v-diff">
            {#each segments($t('conflict.diffHere', { n: changedCount + onlyLocalCount }), { n: changedCount + onlyLocalCount }, ['n']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
          </div>
          {#if onlyLocalCount > 0}
            <div class="v-only">{$t('conflict.onlyHere', { n: onlyLocalCount })}</div>
          {/if}
          <div class="v-stats">{sideSummary(current.localStats)}</div>
          <div class="v-time">{$t('conflict.lastChange', { time: fmtMs(localMs) })}</div>
        </div>
        <div class="version" class:newer={newerSide === 'remote'}>
          <div class="v-head">
            <span class="v-title with-icon"><svelte:component this={peerIcon} size={16} /> {peerName}</span>
            {#if newerSide === 'remote'}<span class="v-badge">{$t('locConflict.changedRecently')}</span>{/if}
          </div>
          <div class="v-diff">
            {#each segments($t('conflict.diffThere', { n: changedCount + onlyRemoteCount }), { n: changedCount + onlyRemoteCount }, ['n']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
          </div>
          {#if onlyRemoteCount > 0}
            <div class="v-only">{$t('conflict.onlyThere', { n: onlyRemoteCount, peer: peerName })}</div>
          {/if}
          <div class="v-stats">{sideSummary(current.remoteStats)}</div>
          <div class="v-time">{$t('conflict.lastChange', { time: fmtMs(remoteMs) })}</div>
        </div>
      </div>

      {#if current.diffTotal > 0}
        <button class="diff-toggle" on:click={() => (showDiff = !showDiff)}>
          <Chevron open={showDiff} /> {$t('conflict.diffToggle', { n: current.diffTotal })}
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
              <div class="diff-more">{$t('conflict.diffMore', { n: current.diffTotal - diffFiles.length })}</div>
            {/if}
          </div>
        {/if}
      {/if}

      <div class="actions">
        <button class="btn ghost later" disabled={busy} on:click={putOff} title={$t('locConflict.decideLaterTitle')}>{$t('conflict.decideLater')}</button>
        <button class="btn" disabled={busy} on:click={() => resolve('keep-remote')}>
          {$t('locConflict.useTheirs', { peer: peerName })}
        </button>
        <button class="btn primary" disabled={busy} on:click={() => resolve('keep-local')}>
          {$t('conflict.keepMine')}
        </button>
      </div>
      <p class="hint-line">
        <History size={15} class="inline-icon" />
        {#each hintUndoParts as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
      </p>
      <p class="hint-line">
        {$t('locConflict.hintNoKeepBoth')}
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
    color: var(--accent-text);
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
  .actions .later {
    margin-right: auto;
  }
  .hint-line {
    margin-top: 12px;
    font-size: 0.78rem;
    color: var(--text-faint);
    line-height: 1.5;
  }
</style>
