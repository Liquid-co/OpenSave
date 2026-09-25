<script>
  // Ctrl+K: type a few letters of a game, a page or an action and press
  // Enter. Everything in it is also somewhere on screen; this is the way to
  // it that does not need the mouse or knowing where it is.
  import { createEventDispatcher, onMount, tick } from 'svelte';
  import Search from 'lucide-svelte/icons/search';
  import CornerDownLeft from 'lucide-svelte/icons/corner-down-left';
  import { navigate, toast, syncPause } from '../lib/stores.js';
  import { PAUSE_CHOICES, pauseSync, resumeSync } from '../lib/syncpause.js';
  import { collections, collectionFilter } from '../lib/collections.js';
  import { api } from '../lib/api.js';
  import { visibleGames, runGameAction } from '../lib/gameactions.js';
  import { appearance, resolvedTheme } from '../lib/appearance.js';
  import { PAGES } from '../lib/shortcuts.js';
  import { rank, snapshotEntries } from '../lib/palette.js';

  const dispatch = createEventDispatcher();
  const close = () => dispatch('close');

  let query = '';
  let active = 0;
  let input;
  let listEl;

  onMount(() => input?.focus());

  const isDark = () => resolvedTheme($appearance.theme, globalThis.matchMedia?.('(prefers-color-scheme: dark)').matches ?? true) === 'dark';

  $: entries = [
    ...PAGES.map((p) => ({ label: `Go to ${p.label}`, kind: 'Page', idle: true, keywords: [p.id], run: () => navigate(p.id) })),
    { label: 'Sync all games', kind: 'Action', idle: true, run: syncAll },
    { label: 'Snapshot every game now', kind: 'Action', idle: true, keywords: ['backup', 'save', 'all'], run: snapshotAll },
    { label: 'Auto-scan for saves', kind: 'Action', idle: true, keywords: ['find', 'detect'], run: () => navigate('home', { scan: Date.now() }) },
    { label: 'Track a folder', kind: 'Action', idle: true, keywords: ['add', 'game'], run: () => navigate('home', { add: true }) },
    {
      label: isDark() ? 'Switch to light theme' : 'Switch to dark theme',
      kind: 'Action',
      idle: true,
      keywords: ['theme', 'appearance', 'mode'],
      run: () => appearance.update((a) => ({ ...a, theme: isDark() ? 'light' : 'dark' }))
    },
    { label: 'Keyboard shortcuts', kind: 'Help', idle: true, keywords: ['keys', 'help'], run: () => dispatch('help') },
    ...($syncPause.paused
      ? [{ label: 'Resume syncing', kind: 'Action', idle: true, keywords: ['unpause', 'start'], run: resumeSync }]
      : PAUSE_CHOICES.map((c) => ({
          label: `Pause syncing ${c.label}`,
          kind: 'Action',
          keywords: ['stop', 'hold', 'offline'],
          run: () => pauseSync(c.minutes)
        }))),
    ...$collections
      .filter((c) => c.gameIds.length)
      .map((c) => ({
        label: `Show ${c.name}`,
        kind: 'Collection',
        idle: c.builtin,
        keywords: ['collection', 'filter'],
        run: () => {
          collectionFilter.set(c.id);
          navigate('home');
        }
      })),
    ...$visibleGames.flatMap((g) => [
      { label: g.name, kind: 'Game', idle: true, weight: 1, run: () => runGameAction('open', g) },
      { label: `Sync ${g.name}`, kind: 'Action', weight: -1, run: () => runGameAction('sync', g) },
      { label: `Snapshot ${g.name}`, kind: 'Action', weight: -1, keywords: ['backup', 'save'], run: () => runGameAction('snapshot', g) },
      ...(g.appId || g.exePath ? [{ label: `Launch ${g.name}`, kind: 'Action', weight: -1, keywords: ['play', 'start'], run: () => runGameAction('launch', g) }] : []),
      { label: `Open ${g.name} save folder`, kind: 'Action', weight: -1, keywords: ['explorer', 'files'], run: () => runGameAction('folder', g) }
    ]),
    // Snapshots with words of their own — a note, or a comment someone typed
    // — found by those words: "boss" finds "Before the final boss".
    ...snapshotEntries($visibleGames).map((s) => ({ ...s, run: () => navigate('game', { gameId: s.gameId, snapshot: s.snapshotId }) }))
  ];

  $: results = rank(entries, query);
  $: if (query !== undefined) active = 0;

  async function snapshotAll() {
    try {
      const res = await api.post('/api/snapshots/all', {});
      toast(
        res.failed?.length ? `Snapshot of ${res.taken} game(s); ${res.failed.length} could not be taken` : `Took a snapshot of ${res.taken} game(s)`,
        res.failed?.length ? 'error' : 'success'
      );
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function syncAll() {
    for (const g of $visibleGames) api.post(`/api/games/${g.id}/sync`).catch(() => {});
    toast('Sync triggered for all games');
  }

  function choose(entry) {
    if (!entry) return;
    close();
    entry.run();
  }

  async function move(delta) {
    if (results.length === 0) return;
    active = (active + delta + results.length) % results.length;
    await tick();
    listEl?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' });
  }

  function onKeydown(e) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      move(1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      move(-1);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      choose(results[active]);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      close();
    }
  }
</script>

<div class="scrim" role="presentation" on:mousedown|self={close}>
  <div class="palette" role="dialog" aria-modal="true" aria-label="Quick switcher">
    <div class="input-row">
      <Search size={17} />
      <input
        bind:this={input}
        bind:value={query}
        on:keydown={onKeydown}
        placeholder="Jump to a game, a page or an action…"
        role="combobox"
        aria-expanded="true"
        aria-controls="palette-results"
        aria-activedescendant={results[active] ? `palette-${active}` : undefined}
        spellcheck="false"
        autocomplete="off"
      />
      <kbd>Esc</kbd>
    </div>
    <ul id="palette-results" role="listbox" bind:this={listEl}>
      {#each results as entry, i}
        <li
          id="palette-{i}"
          role="option"
          aria-selected={i === active}
          class:active={i === active}
          on:mousemove={() => (active = i)}
          on:mousedown|preventDefault={() => choose(entry)}
        >
          <span class="label">{entry.label}</span>
          <span class="kind">{entry.kind}</span>
          {#if i === active}<CornerDownLeft size={13} />{/if}
        </li>
      {:else}
        <li class="none">Nothing matches “{query}”.</li>
      {/each}
    </ul>
  </div>
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 150;
    background: var(--overlay);
    display: flex;
    justify-content: center;
    align-items: flex-start;
    padding: 12vh 20px 20px;
  }
  .palette {
    width: min(600px, 100%);
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
    overflow: hidden;
    animation: drop 0.12s ease-out;
  }
  @keyframes drop {
    from {
      opacity: 0;
      transform: translateY(-6px);
    }
  }
  .input-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 14px;
    border-bottom: 1px solid var(--border);
    color: var(--text-faint);
  }
  input {
    flex: 1;
    padding: 15px 0;
    border: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
    outline: none;
  }
  kbd {
    font: inherit;
    font-size: 0.72rem;
    padding: 2px 6px;
    border: 1px solid var(--border-strong);
    border-radius: 5px;
    color: var(--text-faint);
  }
  ul {
    list-style: none;
    max-height: min(52vh, 420px);
    overflow-y: auto;
    padding: 6px;
  }
  li {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 12px;
    border-radius: 8px;
    font-size: 0.92rem;
    color: var(--text);
    cursor: pointer;
  }
  li.active {
    background: var(--accent-soft);
  }
  .label {
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .kind {
    font-size: 0.74rem;
    color: var(--text-faint);
  }
  li :global(svg) {
    color: var(--text-dim);
  }
  li.none {
    color: var(--text-faint);
    cursor: default;
    justify-content: center;
  }
</style>
