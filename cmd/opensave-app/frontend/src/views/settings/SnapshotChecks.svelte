<script>
  import { t as translateUI } from '../../lib/i18n.js';
  // Whether every snapshot can still be restored: when they were last read
  // back, what was found, what to do about what was found, and how often it
  // happens by itself. See daemon/verify.go and daemon/repair.go.
  import { onMount } from 'svelte';
  import FileCheck from 'lucide-svelte/icons/file-check';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import CloudDownload from 'lucide-svelte/icons/cloud-download';
  import Trash2 from 'lucide-svelte/icons/trash-2';
  import { api } from '../../lib/api.js';
  import { games, navigate, toast, askConfirm, settings } from '../../lib/stores.js';
  import { timeAgo } from '../../lib/timeago.js';
  import { whenLabel } from '../../lib/snapshots.js';

  let summary = null;
  let checking = false;
  let repairing = false;
  let removing = false;
  let details = false;

  async function load() {
    try {
      summary = await api.get('/api/snapshots/check');
    } catch {
      summary = null;
    }
  }
  onMount(load);

  async function checkNow() {
    checking = true;
    try {
      const report = await api.post('/api/snapshots/check', {});
      summary = report.summary;
      const fixed = report.repaired ? $translateUI('snapshotChecks.putBackFromTheCloud', { p0: report.repaired }) : '';
      toast(
        report.damaged.length
          ? $translateUI('snapshotChecks.ofSnapshotsCanTBeRestored', { p0: report.damaged.length, p1: report.checked, p2: fixed })
          : $translateUI('snapshotChecks.checkedSnapshotsEveryOneCanBe', { p0: report.checked, p1: fixed }),
        report.damaged.length ? 'error' : 'success'
      );
      settings.update((s) => (s ? { ...s, lastVerifyMs: Date.now() } : s));
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      checking = false;
    }
  }

  // Their cloud copies, where there are whole ones: fetched, checked, and
  // put back where the archives were.
  async function repair() {
    repairing = true;
    try {
      const r = await api.post('/api/snapshots/repair', {});
      const back = r.repaired.length;
      const left = r.remaining.length;
      if (!r.cloudChecked) {
        toast($translateUI('snapshotChecks.couldnTLookInTheCloud', { p0: r.cloudError }), 'error');
      } else if (back && left) {
        toast($translateUI('snapshotChecks.putBackFromTheCloudHaveNo', { p0: back, p1: left }), 'success');
      } else if (back) {
        toast($translateUI('snapshotChecks.putBackAllFromTheCloud', { p0: back }), 'success');
      } else {
        toast($translateUI('snapshotChecks.noneOfThemHaveACopyIn'), 'info');
      }
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      repairing = false;
      load();
    }
  }

  async function removeDamaged() {
    const n = summary.damaged.length;
    const ok = await askConfirm(
      $translateUI('snapshotChecks.removeTheSnapshotThatCanTBeRestored', { p0: n, p1: n === 1 ? '' : 's' }),
      { title: $translateUI('snapshotChecks.removeThem'), confirmText: $translateUI('snapshotChecks.remove', { p0: n }), danger: true }
    );
    if (!ok) return;
    removing = true;
    try {
      const r = await api.post('/api/snapshots/forget-damaged', {});
      toast($translateUI('snapshotChecks.removedSnapshotThatCouldnTBeRestored', { p0: r.removed.length, p1: r.removed.length === 1 ? '' : 's' }), 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      removing = false;
      load();
    }
  }

  // How often it happens by itself: a daemon setting (verify_every_days).
  $: EVERY = [
    [1, $translateUI('snapshotChecks.everyDay')],
    [3, $translateUI('snapshotChecks.every3Days')],
    [7, $translateUI('snapshotChecks.everyWeek')],
    [14, $translateUI('snapshotChecks.every2Weeks')],
    [30, $translateUI('snapshotChecks.everyMonth')]
  ];
  async function setEvery(days) {
    try {
      const saved = await api.post('/api/settings', { verifyEveryDays: days });
      settings.set(saved?.deviceName !== undefined ? saved : { ...$settings, verifyEveryDays: days });
    } catch (e) {
      toast(e.message, 'error');
    }
  }
  $: every = $settings?.verifyEveryDays ?? 7;
  $: auto = every > 0;
  let lastEvery = 7;
  $: if (every > 0) lastEvery = every;
  $: nextAt = auto && $settings?.lastVerifyMs ? $settings.lastVerifyMs + every * 86_400_000 : 0;

  const iso = (ms) => new Date(ms).toISOString();
  const nameOf = (id) => $games[id]?.name ?? id;
  // The damaged, by game: a list of every row reads as a wall of paths.
  $: byGame = Object.entries(
    (summary?.damaged ?? []).reduce((acc, s) => {
      (acc[s.gameId] ??= []).push(s);
      return acc;
    }, {})
  ).sort((a, b) => b[1].length - a[1].length);
  function inWords(ms) {
    const days = Math.round((ms - Date.now()) / 86_400_000);
    if (days <= 0) return 'soon';
    return days === 1 ? 'tomorrow' : $translateUI('snapshotChecks.inDays', { p0: days });
  }
</script>

<div class="checks">
  {#if summary}
    <div class="state" class:bad={summary.damaged.length}>
      {#if summary.damaged.length}
        <TriangleAlert size={18} />
        <span>
          <strong>{summary.damaged.length} {summary.damaged.length === 1 ? $translateUI('snapshotChecks.snapshotCanT') : $translateUI('snapshotChecks.snapshotsCanT')} {$translateUI('snapshotChecks.beRestored')}</strong>
          {$translateUI('snapshotChecks.theirArchivesWereDeletedOrDamagedOutside')}
        </span>
      {:else if summary.lastCheckedMs}
        <FileCheck size={18} />
        <span>
          <strong>{$translateUI('snapshotChecks.everySnapshotCheckedCanBeRestored')}</strong>
          {summary.checked} {$translateUI('cloudBrowser.of')} {summary.total} {$translateUI('snapshotChecks.checkedLast')} {timeAgo(iso(summary.lastCheckedMs))}.
        </span>
      {:else}
        <FileCheck size={18} />
        <span>{$translateUI('snapshotChecks.notCheckedYet')}</span>
      {/if}
    </div>

    {#if summary.damaged.length}
      <ul class="bygame">
        {#each byGame as [gameId, list] (gameId)}
          <li>
            <button class="linklike" on:click={() => navigate('game', { gameId, snapshot: list[0].id })}>{nameOf(gameId)}</button>
            <span class="count">{list.length} {$translateUI('cloudBrowser.snapshot')}{list.length === 1 ? '' : 's'}, {whenLabel(list[list.length - 1].timestamp)}{list.length > 1 ? $translateUI('snapshotChecks.to', { p0: whenLabel(list[0].timestamp) }) : ''}</span>
          </li>
        {/each}
      </ul>
      <div class="fix">
        <button class="btn small primary" disabled={repairing || removing} on:click={repair}>
          <CloudDownload size={14} />{repairing ? $translateUI('snapshotChecks.lookingInTheCloud') : $translateUI('snapshotChecks.lookForCopiesInTheCloud')}
        </button>
        <button class="btn small" disabled={repairing || removing} on:click={removeDamaged}>
          <Trash2 size={14} />{removing ? $translateUI('snapshotChecks.removing') : $translateUI('snapshotChecks.removeThemFromTheHistory')}
        </button>
        <button class="linklike quiet" on:click={() => (details = !details)}>{details ? $translateUI('snapshotChecks.hideDetails') : $translateUI('snapshotChecks.details')}</button>
      </div>
      <p class="hint">
        {$translateUI('snapshotChecks.withCloudBackupOnEachSnapshotWas')}
      </p>
      {#if details}
        <ul class="damaged">
          {#each summary.damaged as s (s.id)}
            <li>
              <button class="linklike" on:click={() => navigate('game', { gameId: s.gameId, snapshot: s.id })}>
                {nameOf(s.gameId)} — {whenLabel(s.timestamp)}
              </button>
              <span class="why">{s.problem}</span>
            </li>
          {/each}
        </ul>
      {/if}
    {/if}
  {/if}

  <div class="schedule">
    <label class="toggle">
      <input type="checkbox" checked={auto} on:change={(e) => setEvery(e.currentTarget.checked ? lastEvery : 0)} />
      {$translateUI('snapshotChecks.checkAutomatically')}
    </label>
    {#if auto}
      <select value={every} on:change={(e) => setEvery(Number(e.currentTarget.value))} aria-label="{$translateUI('snapshotChecks.howOften')}">
        {#each EVERY as [days, label]}
          <option value={days}>{label}</option>
        {/each}
        {#if !EVERY.some(([d]) => d === every)}<option value={every}>{$translateUI('snapshotChecks.every')} {every} {$translateUI('snapshotChecks.days')}</option>{/if}
      </select>
      <span class="hint">{nextAt ? $translateUI('snapshotChecks.next', { p0: inWords(nextAt) }) : $translateUI('snapshotChecks.theFirstALittleAfterOpenSaveStarts')} {$translateUI('snapshotChecks.anythingTheCloudStillHasWholeIs')}</span>
    {:else}
      <span class="hint">{$translateUI('snapshotChecks.snapshotsAreStillCheckedBeforeTheyReRestored')}</span>
    {/if}
  </div>

  <div class="actions">
    <button class="btn small" disabled={checking} on:click={checkNow}>{checking ? $translateUI('snapshotChecks.checking') : $translateUI('snapshotChecks.checkEverySnapshotNow')}</button>
    <span class="hint">{$translateUI('snapshotChecks.eachIsReadBackInFullAnd')}</span>
  </div>
</div>

<style>
  .checks {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .state {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    font-size: 0.86rem;
    color: var(--text-dim);
  }
  .state :global(svg) {
    flex-shrink: 0;
    margin-top: 1px;
    color: var(--success);
  }
  .state.bad :global(svg) {
    color: var(--danger);
  }
  .state strong {
    color: var(--text);
  }
  .bygame {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.84rem;
  }
  .bygame li {
    display: flex;
    gap: 10px;
    align-items: baseline;
  }
  .count {
    color: var(--text-faint);
    font-size: 0.8rem;
  }
  .fix {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }
  .fix + .hint {
    margin: -4px 0 0;
  }
  .damaged {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 0.82rem;
  }
  .damaged li {
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .why {
    color: var(--danger-text);
    font-size: 0.78rem;
    word-break: break-all;
  }
  .linklike {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    color: var(--text);
    text-align: left;
    cursor: pointer;
    text-decoration: underline;
  }
  .linklike.quiet {
    color: var(--text-faint);
    font-size: 0.82rem;
  }
  .schedule {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
  }
  .toggle {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: 0.86rem;
    cursor: pointer;
  }
  .schedule select {
    padding: 6px 10px;
    background-color: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.85rem;
  }
  .schedule .hint,
  .actions .hint {
    margin: 0;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
  }
</style>
