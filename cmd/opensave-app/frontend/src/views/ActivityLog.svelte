<script>
  import { onDestroy } from 'svelte';
  import { logEntries, gameList, navigate, toast } from '../lib/stores.js';
  import { asText, feedByDay, filterEntries, linkGames } from '../lib/activity.js';
  import SearchInput from '../components/ui/SearchInput.svelte';
  import FilterTabs from '../components/ui/FilterTabs.svelte';

  // Remembered on this device only; nothing depends on them.
  const MODE_KEY = 'opensave.activityMode';
  let technical = false;
  try {
    technical = localStorage.getItem(MODE_KEY) === 'technical';
  } catch {}
  $: try {
    localStorage.setItem(MODE_KEY, technical ? 'technical' : 'feed');
  } catch {}

  let show = 'highlights';
  let query = '';
  $: problemCount = filterEntries($logEntries, { show: 'problems' }).length;
  $: shown = filterEntries($logEntries, { show, query });
  $: groups = feedByDay(shown, now);

  let now = new Date();
  const tick = setInterval(() => (now = new Date()), 60_000);
  onDestroy(() => clearInterval(tick));

  const icons = { success: '✓', info: '•', warn: '!', error: '✕' };
  const fmtClock = (t) => new Date(t).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
  const fmtStamp = (t) => new Date(t).toLocaleTimeString();

  async function copyShown() {
    try {
      await navigator.clipboard.writeText(asText(shown));
      toast(`Copied ${shown.length} ${shown.length === 1 ? 'entry' : 'entries'}`, 'success');
    } catch (e) {
      toast(`Could not copy: ${e.message}`, 'error');
    }
  }

  // The technical log sticks to the bottom as new entries arrive, like a
  // terminal. Implemented as an action (node is a local, uninstrumented
  // reference): assigning scrollTop on a bind:this variable would
  // $$invalidate it and loop the reactive flush forever, freezing the app.
  function autoscroll(node, _count) {
    const toBottom = () => queueMicrotask(() => (node.scrollTop = node.scrollHeight));
    toBottom();
    return { update: toBottom };
  }
</script>

<div class="head">
  <h2 class="page-title">Activity</h2>
  <div class="controls">
    <FilterTabs
      options={[['highlights', 'Highlights'], ['all', 'Everything'], ['problems', 'Problems']]}
      counts={{ problems: problemCount }}
      bind:value={show}
    />
    <SearchInput placeholder="Search activity…" bind:value={query} />
    <label class="toggle" title="Every entry exactly as logged, oldest first — what a bug report wants">
      <input type="checkbox" bind:checked={technical} />
      Technical log
    </label>
    <button class="btn small" disabled={shown.length === 0} on:click={copyShown} title="Copy what is shown, for a bug report">
      Copy
    </button>
  </div>
</div>

{#if $logEntries.length === 0}
  <div class="card"><div class="empty"><h3>Nothing yet</h3><p>Syncs, snapshots and anything that needs your attention show up here.</p></div></div>
{:else if shown.length === 0}
  <div class="card"><div class="empty"><h3>Nothing matches</h3><p>
        {#if query}Try another search.{:else if show === 'problems'}No warnings or errors — all quiet.{:else}Nothing but routine so far — <button class="linklike" on:click={() => (show = 'all')}>show everything</button>.{/if}
      </p></div></div>
{:else if technical}
  <div class="card log" use:autoscroll={shown.length}>
    {#each shown as entry}
      <div class="line">
        <span class="stamp">{fmtStamp(entry.timestamp)}</span>
        <span class="level lvl-{entry.level}">{entry.level}</span>
        <span class="raw">{entry.message}</span>
      </div>
    {/each}
  </div>
{:else}
  <div class="feed">
    {#each groups as group (group.day)}
      <h4 class="day">{group.day}</h4>
      <div class="card entries">
        {#each group.entries as entry}
          <div class="entry lvl-{entry.level}">
            <span class="icon" aria-label={entry.level}>{icons[entry.level] ?? '•'}</span>
            <span class="msg">
              {#each linkGames(entry.message, $gameList) as part}
                {#if part.gameId}
                  <button class="game" on:click={() => navigate('game', { gameId: part.gameId })}>{part.text}</button>
                {:else}{part.text}{/if}
              {/each}
            </span>
            <span class="clock" title={new Date(entry.timestamp).toLocaleString()}>{fmtClock(entry.timestamp)}</span>
          </div>
        {/each}
      </div>
    {/each}
  </div>
{/if}

<style>
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
    margin-bottom: 18px;
  }
  .controls {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 0.85rem;
    color: var(--text-dim);
    cursor: pointer;
    white-space: nowrap;
  }
  .toggle input {
    width: 15px;
    height: 15px;
  }
  .linklike {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    color: var(--accent);
    cursor: pointer;
  }

  /* Feed */
  .feed {
    display: flex;
    flex-direction: column;
  }
  .day {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-faint);
    margin: 18px 0 8px;
  }
  .day:first-child {
    margin-top: 0;
  }
  .entries {
    padding: 4px 0;
  }
  .entry {
    --tone: var(--text-faint);
    --tone-bg: rgba(158, 158, 168, 0.12);
    display: flex;
    align-items: baseline;
    gap: 12px;
    padding: 8px 16px;
    font-size: 0.88rem;
    line-height: 1.45;
  }
  .entry + .entry {
    border-top: 1px solid var(--border);
  }
  .entry.lvl-success {
    --tone: var(--success);
    --tone-bg: rgba(74, 222, 128, 0.13);
  }
  .entry.lvl-warn {
    --tone: var(--warn);
    --tone-bg: rgba(251, 191, 36, 0.13);
  }
  .entry.lvl-error {
    --tone: var(--danger);
    --tone-bg: rgba(217, 87, 87, 0.16);
  }
  .icon {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    align-self: center;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    font-size: 0.7rem;
    font-weight: 800;
    color: var(--tone);
    background: var(--tone-bg);
  }
  .msg {
    flex: 1;
    min-width: 0;
    color: var(--text);
    word-break: break-word;
    user-select: text;
  }
  .lvl-warn .msg,
  .lvl-error .msg {
    color: var(--tone);
  }
  .game {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-weight: 600;
    color: inherit;
    cursor: pointer;
    text-decoration: underline;
    text-decoration-color: var(--border-strong);
    text-underline-offset: 2px;
  }
  .game:hover {
    color: var(--accent-hover);
    text-decoration-color: currentColor;
  }
  .clock {
    flex-shrink: 0;
    font-size: 0.75rem;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }

  /* Technical log: the entries exactly as written, oldest first. */
  .log {
    height: calc(100vh - var(--titlebar-h) - var(--statusbar-h) - 130px);
    overflow-y: auto;
    font-family: 'Cascadia Code', 'Consolas', monospace;
    font-size: 0.8rem;
    padding: 14px;
  }
  .line {
    display: flex;
    gap: 10px;
    padding: 2px 0;
    align-items: baseline;
  }
  .stamp {
    color: var(--text-faint);
    flex-shrink: 0;
  }
  .level {
    width: 56px;
    flex-shrink: 0;
    font-weight: 600;
    color: var(--text-dim);
  }
  .level.lvl-success {
    color: var(--success);
  }
  .level.lvl-warn {
    color: var(--warn);
  }
  .level.lvl-error {
    color: var(--danger);
  }
  .raw {
    color: var(--text);
    word-break: break-word;
    user-select: text;
  }
</style>
