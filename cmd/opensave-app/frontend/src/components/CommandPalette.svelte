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
  import { visibleGames, runGameAction, canLaunch } from '../lib/gameactions.js';
  import { showSetupAgain } from '../lib/setup.js';
  import { appearance, resolvedTheme } from '../lib/appearance.js';
  import { PAGES } from '../lib/shortcuts.js';
  import { t as translateUI } from '../lib/i18n.js';
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
    ...PAGES.map((p) => ({ label: $translateUI('palette.goTo', { page: p.label }), kind: 'Page', idle: true, keywords: [p.id], run: () => navigate(p.id) })),
    { label: $translateUI('palette.syncAll'), kind: 'Action', idle: true, run: syncAll },
    { label: $translateUI('palette.snapshotAll'), kind: 'Action', idle: true, keywords: ['backup', 'save', 'all'], run: snapshotAll },
    { label: $translateUI('home.welcome.scanForSaves'), kind: 'Action', idle: true, keywords: ['find', 'detect', 'auto-scan'], run: () => navigate('home', { scan: Date.now() }) },
    { label: $translateUI('palette.trackFolder'), kind: 'Action', idle: true, keywords: ['add', 'game'], run: () => navigate('home', { add: true }) },
    {
      label: isDark() ? $translateUI('palette.toLight') : $translateUI('palette.toDark'),
      kind: 'Action',
      idle: true,
      keywords: ['theme', 'appearance', 'mode'],
      run: () => appearance.update((a) => ({ ...a, theme: isDark() ? 'light' : 'dark' }))
    },
    { label: $translateUI('shortcuts.title'), kind: 'Help', idle: true, keywords: ['keys', 'help'], run: () => dispatch('help') },
    {
      label: $translateUI('palette.showSetup'),
      kind: 'Help',
      keywords: ['tutorial', 'getting started', 'onboarding', 'help', 'checklist'],
      run: () => {
        showSetupAgain();
        navigate('home');
      }
    },
    ...($syncPause.paused
      ? [{ label: $translateUI('pause.resume'), kind: 'Action', idle: true, keywords: ['unpause', 'start'], run: resumeSync }]
      : PAUSE_CHOICES.map((c) => ({
          label: $translateUI('palette.pauseFor', { when: c.label }),
          kind: 'Action',
          keywords: ['stop', 'hold', 'offline'],
          run: () => pauseSync(c.minutes)
        }))),
    ...$collections
      .filter((c) => c.gameIds.length)
      .map((c) => ({
        label: $translateUI('palette.show', { name: c.name }),
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
      { label: $translateUI('palette.syncGame', { game: g.name }), kind: 'Action', weight: -1, run: () => runGameAction('sync', g) },
      { label: $translateUI('palette.snapshotGame', { game: g.name }), kind: 'Action', weight: -1, keywords: ['backup', 'save'], run: () => runGameAction('snapshot', g) },
      ...(canLaunch(g) ? [{ label: $translateUI('palette.launchGame', { game: g.name }), kind: 'Action', weight: -1, keywords: ['play', 'start'], run: () => runGameAction('launch', g) }] : []),
      { label: $translateUI('palette.openSaveFolder', { game: g.name }), kind: 'Action', weight: -1, keywords: ['explorer', 'files'], run: () => runGameAction('folder', g) },
      { label: $translateUI('palette.renameGame', { game: g.name }), kind: 'Action', weight: -1, keywords: ['name', 'title'], run: () => runGameAction('rename', g) }
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
        res.failed?.length
          ? $translateUI('palette.snapshotAllPartial', { n: res.taken, failed: res.failed.length })
          : $translateUI('palette.snapshotAllDone', { n: res.taken }),
        res.failed?.length ? 'error' : 'success'
      );
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function syncAll() {
    for (const g of $visibleGames) api.post(`/api/games/${g.id}/sync`).catch(() => {});
    toast($translateUI('home.syncAllToast'));
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
  <div class="palette" role="dialog" aria-modal="true" aria-label={$translateUI('palette.ariaLabel')}>
    <div class="input-row">
      <Search size={17} />
      <input
        bind:this={input}
        bind:value={query}
        on:keydown={onKeydown}
        placeholder={$translateUI('palette.placeholder')}
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
          <span class="label">{($translateUI, entry.label)}</span>
          <span class="kind">{$translateUI(`palette.kind.${entry.kind.toLowerCase()}`)}</span>
          {#if i === active}<CornerDownLeft size={13} />{/if}
        </li>
      {:else}
        <li class="none">{$translateUI('palette.noMatches', { query })}</li>
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
