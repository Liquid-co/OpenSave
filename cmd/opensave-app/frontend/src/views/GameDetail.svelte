<script>
  // A game's page: its own art behind the top, the game and where its save
  // stands (GameHero), where the save has been over two weeks (SaveTimeline),
  // then four tabs: History, Branches, Devices, Settings.
  //
  // The page can take its colours from the cover (Settings → Appearance,
  // lib/covertint.js): Play, the tab underline and the parts marked as this
  // game's carry --game-* variables, the accent where there are none.
  import { onDestroy } from 'svelte';
  import { t as i18n } from '../lib/i18n.js';
  import { slidingIndicator } from '../lib/motion.js';
  import { games, peers, settings, activityTick, toast } from '../lib/stores.js';
  import { api, gameCover } from '../lib/api.js';
  import { appearance } from '../lib/appearance.js';
  import { coverTint, tintVars } from '../lib/covertint.js';
  import { deviceSlots } from '../lib/activityview.js';
  import { createRunner } from '../lib/runner.js';
  import { splitMessage } from '../lib/toasts.js';
  import LibraryBackdrop from './home/LibraryBackdrop.svelte';
  import GameHero from './game/GameHero.svelte';
  import SaveTimeline from './game/SaveTimeline.svelte';
  import SnapshotsTab from './game/SnapshotsTab.svelte';
  import BranchesTab from './game/BranchesTab.svelte';
  import DevicesTab from './game/DevicesTab.svelte';
  import SettingsTab from './game/SettingsTab.svelte';
  import Camera from 'lucide-svelte/icons/camera';

  export let params = {};

  $: game = $games[params.gameId];

  // One action at a time across the whole page — see lib/runner.js. Its
  // toasts carry the game: its cover, and its name under a title that does
  // not say it already.
  const runner = createRunner((message, kind, shown = {}) =>
    toast(message, kind, {
      game: params.gameId,
      ...(game && !splitMessage(message).detail && !shown.detail ? { detail: game.name } : {}),
      ...shown
    })
  );

  const tabs = [
    { id: 'history', label: 'game.tab.history', component: SnapshotsTab },
    { id: 'branches', label: 'game.tab.branches', component: BranchesTab },
    { id: 'devices', label: 'game.tab.devices', component: DevicesTab },
    { id: 'settings', label: 'game.tab.settings', component: SettingsTab }
  ];
  let tab = 'history';
  let focusSnap = null;
  let takeRequest = 0;

  // A tab is built the first time it is opened and then kept, hidden, while
  // another is shown: half-typed settings should still be there after a look
  // at another tab. The page is reused when moving between games, so all of
  // it starts over when the game changes.
  let opened = new Set([tab]);
  $: if (params.snapshot) {
    tab = 'history';
    focusSnap = params.snapshot;
  }
  let shownFor = params.gameId;
  $: if (params.gameId !== shownFor) {
    shownFor = params.gameId;
    tab = 'history';
    focusSnap = params.snapshot ?? null;
    opened = new Set([tab]);
    report = null;
  }
  $: if (!opened.has(tab)) opened = new Set(opened).add(tab);

  // ── What Activity knows of it: its sessions on every device ──
  let report = null;
  async function readReport(id) {
    try {
      const r = await api.get(`/api/activity?game=${encodeURIComponent(id)}&limit=500`);
      if (id === params.gameId) report = r;
    } catch {
      // The page stands without it: the timeline shows only snapshots.
    }
  }
  $: if (params.gameId) readReport(params.gameId, $activityTick);
  $: played = report?.games?.find((g) => g.gameId === params.gameId) ?? null;
  $: here = report?.device || $settings?.deviceName || '';
  $: slots = deviceSlots(here, Object.values($peers), [...new Set((report?.items ?? []).map((i) => i.device).filter(Boolean))]);

  // ── Its colours ──
  let tint = null;
  $: coverUrl = game && $appearance.coverColours && !game.coverExplicit ? gameCover(game, true) : '';
  $: readTint(coverUrl);
  async function readTint(url) {
    if (!url) {
      tint = null;
      return;
    }
    const hex = await coverTint(url);
    if (url === coverUrl) tint = hex;
  }
  let theme = document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
  const unsubTheme = appearance.subscribe(() => {
    queueMicrotask(() => (theme = document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark'));
  });
  onDestroy(unsubTheme);
  $: vars = tint
    ? Object.entries(tintVars(tint, theme))
        .map(([k, v]) => `${k}: ${v}`)
        .join('; ')
    : '';

  $: snapCount = game ? Object.values(game.branches ?? {}).reduce((n, b) => n + (b.snapshots?.length ?? 0), 0) : 0;
  $: counts = { history: snapCount, branches: game ? Object.keys(game.branches ?? {}).length : 0 };

  function takeSnapshot() {
    tab = 'history';
    takeRequest = Date.now();
  }
</script>

{#if !game}
  <div class="empty"><h3>{$i18n('game.notFound')}</h3></div>
{:else}
  <div class="game" style={vars}>
    <LibraryBackdrop {game} height={560} />
    {#key params.gameId}
      <GameHero {game} {runner} {played} renameRequest={params.rename} on:tab={(e) => (tab = e.detail)} on:snapshot={takeSnapshot} />

      <div class="timeline-wrap">
        <SaveTimeline
          {game}
          items={report?.items ?? []}
          {here}
          {slots}
          on:snapshot={(e) => {
            tab = 'history';
            focusSnap = e.detail;
          }}
          on:branches={() => (tab = 'branches')}
        />
      </div>

      <div class="tabbar">
        <div class="tabs" role="tablist" use:slidingIndicator={{ mode: 'underline' }}>
          {#each tabs as tb (tb.id)}
            <button role="tab" aria-selected={tab === tb.id} class:active={tab === tb.id} on:click={() => (tab = tb.id)}>
              {$i18n(tb.label)}{#if counts[tb.id] !== undefined}<span class="count">{counts[tb.id]}</span>{/if}
            </button>
          {/each}
        </div>
        {#if tab === 'history'}
          <button class="btn small take" on:click={takeSnapshot}><Camera size={15} />{$i18n('game.hero.takeSnapshot')}</button>
        {/if}
      </div>

      {#each tabs as tb (tb.id)}
        {#if opened.has(tb.id)}
          <div class="panel" hidden={tab !== tb.id} role="tabpanel">
            <svelte:component
              this={tb.component}
              {game}
              {runner}
              {...tb.id === 'history' ? { focus: focusSnap, takeRequest } : tb.id === 'branches' ? { here } : {}}
            />
          </div>
        {/if}
      {/each}
    {/key}
  </div>
{/if}

<style>
  .game {
    position: relative;
    isolation: isolate;
    max-width: 1400px;
  }
  .timeline-wrap {
    margin-top: 18px;
    animation: arrive 0.4s cubic-bezier(0.2, 0.7, 0.2, 1) 0.12s backwards;
  }
  .tabbar {
    display: flex;
    align-items: center;
    gap: 12px;
    margin: 20px 0 16px;
    border-bottom: 1px solid var(--border);
    animation: arrive 0.4s cubic-bezier(0.2, 0.7, 0.2, 1) 0.18s backwards;
  }
  .tabs {
    flex: 1;
    display: flex;
    gap: 26px;
  }
  .tabs button {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    height: 44px;
    padding: 0 2px;
    border: 0;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-base);
    cursor: pointer;
    transition: color 0.15s;
  }
  .tabs button:hover {
    color: var(--text);
  }
  .tabs button.active {
    color: var(--text);
    font-weight: 600;
  }
  /* The moving underline, in the game's colour. */
  .tabs :global(.slide-underline) {
    height: 2px;
    background: var(--game-accent, var(--accent));
  }
  .count {
    min-width: 20px;
    padding: 0 6px;
    border-radius: 999px;
    border: 1px solid var(--border-strong);
    font-size: var(--text-xs);
    font-weight: 600;
    line-height: 1.6;
    color: var(--text-dim);
    text-align: center;
  }
  .take {
    margin-bottom: 6px;
  }
  /* A tab arriving: a short rise, as the page does. */
  .panel:not([hidden]) {
    animation: arrive 0.26s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
  }
</style>
