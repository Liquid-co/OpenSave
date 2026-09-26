<script>
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
      const fixed = report.repaired ? ` — ${report.repaired} put back from the cloud` : '';
      toast(
        report.damaged.length
          ? `${report.damaged.length} of ${report.checked} snapshots can't be restored${fixed}`
          : `Checked ${report.checked} snapshots — every one can be restored${fixed}`,
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
        toast(`Couldn't look in the cloud: ${r.cloudError}`, 'error');
      } else if (back && left) {
        toast(`Put back ${back} from the cloud; ${left} have no copy there`, 'success');
      } else if (back) {
        toast(`Put back all ${back} from the cloud`, 'success');
      } else {
        toast(`None of them have a copy in the cloud`, 'info');
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
      `Remove the ${n} snapshot${n === 1 ? '' : 's'} that can't be restored from the history? Their archives are already gone or damaged, so nothing that could be restored is lost; every other snapshot stays.`,
      { title: 'Remove them?', confirmText: `Remove ${n}`, danger: true }
    );
    if (!ok) return;
    removing = true;
    try {
      const r = await api.post('/api/snapshots/forget-damaged', {});
      toast(`Removed ${r.removed.length} snapshot${r.removed.length === 1 ? '' : 's'} that couldn't be restored`, 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      removing = false;
      load();
    }
  }

  // How often it happens by itself: a daemon setting (verify_every_days).
  const EVERY = [
    [1, 'every day'],
    [3, 'every 3 days'],
    [7, 'every week'],
    [14, 'every 2 weeks'],
    [30, 'every month']
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
    return days === 1 ? 'tomorrow' : `in ${days} days`;
  }
</script>

<div class="checks">
  {#if summary}
    <div class="state" class:bad={summary.damaged.length}>
      {#if summary.damaged.length}
        <TriangleAlert size={18} />
        <span>
          <strong>{summary.damaged.length} {summary.damaged.length === 1 ? "snapshot can't" : "snapshots can't"} be restored.</strong>
          Their archives were deleted or damaged outside OpenSave. Every other snapshot is fine, and so is each game's save.
        </span>
      {:else if summary.lastCheckedMs}
        <FileCheck size={18} />
        <span>
          <strong>Every snapshot checked can be restored.</strong>
          {summary.checked} of {summary.total} checked, last {timeAgo(iso(summary.lastCheckedMs))}.
        </span>
      {:else}
        <FileCheck size={18} />
        <span>Not checked yet.</span>
      {/if}
    </div>

    {#if summary.damaged.length}
      <ul class="bygame">
        {#each byGame as [gameId, list] (gameId)}
          <li>
            <button class="linklike" on:click={() => navigate('game', { gameId, snapshot: list[0].id })}>{nameOf(gameId)}</button>
            <span class="count">{list.length} snapshot{list.length === 1 ? '' : 's'}, {whenLabel(list[list.length - 1].timestamp)}{list.length > 1 ? ` to ${whenLabel(list[0].timestamp)}` : ''}</span>
          </li>
        {/each}
      </ul>
      <div class="fix">
        <button class="btn small primary" disabled={repairing || removing} on:click={repair}>
          <CloudDownload size={14} />{repairing ? 'Looking in the cloud…' : 'Look for copies in the cloud'}
        </button>
        <button class="btn small" disabled={repairing || removing} on:click={removeDamaged}>
          <Trash2 size={14} />{removing ? 'Removing…' : 'Remove them from the history'}
        </button>
        <button class="linklike quiet" on:click={() => (details = !details)}>{details ? 'Hide details' : 'Details'}</button>
      </div>
      <p class="hint">
        With cloud backup on, each snapshot was uploaded as it was taken: a whole copy there is fetched, checked and put back.
        What has no copy anywhere can only be removed — the record promises a save that no longer exists.
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
      Check automatically
    </label>
    {#if auto}
      <select value={every} on:change={(e) => setEvery(Number(e.currentTarget.value))} aria-label="How often">
        {#each EVERY as [days, label]}
          <option value={days}>{label}</option>
        {/each}
        {#if !EVERY.some(([d]) => d === every)}<option value={every}>every {every} days</option>{/if}
      </select>
      <span class="hint">{nextAt ? `Next ${inWords(nextAt)}.` : 'The first a little after OpenSave starts.'} Anything the cloud still has whole is put back as it's found.</span>
    {:else}
      <span class="hint">Snapshots are still checked before they're restored.</span>
    {/if}
  </div>

  <div class="actions">
    <button class="btn small" disabled={checking} on:click={checkNow}>{checking ? 'Checking…' : 'Check every snapshot now'}</button>
    <span class="hint">Each is read back in full and every file compared with its checksum. A snapshot is also checked before it is restored.</span>
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
