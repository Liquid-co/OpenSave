<script>
  // The auto-scan: every save folder the daemon can find, grouped per game,
  // with the ones already tracked set apart.
  //
  // Opened with start(). `scanning` is bindable so the page can disable the
  // buttons that open it while a scan runs.
  import { gameList, toast, askConfirm, settings } from '../../lib/stores.js';
  import { api, coverURL } from '../../lib/api.js';
  import Modal from '../../components/ui/Modal.svelte';
  import Chevron from '../../components/ui/Chevron.svelte';
  import ScanSearch from 'lucide-svelte/icons/scan-search';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Joystick from 'lucide-svelte/icons/joystick';
  import Package from 'lucide-svelte/icons/package';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import ModalLoading from '../../components/ui/ModalLoading.svelte';
  import FilterTabs from '../../components/ui/FilterTabs.svelte';
  import SearchInput from '../../components/ui/SearchInput.svelte';
  import CoverTile from '../../components/ui/CoverTile.svelte';
  import ScanFolderList from './ScanFolderList.svelte';
  // The scan screen's decisions live in a plain module so they can be tested
  // without opening the app — see scan.test.js, where each case is a mistake
  // that actually reached a build.
  import {
    buildGroups,
    contentsLabel,
    fillMissingAppIds,
    isEmptyResult,
    normPath,
    plannedGames,
    rootNameFor
  } from '../../lib/scan.js';

  export let scanning = false;

  let open = false;
  let scanResults = null;
  let scanFilter = '';
  let scanType = 'all';
  let selected = new Set();
  let selectedCount = 0; // reactive mirror of selected.size
  let showTracked = false; // include saves already being tracked
  // Folders with nothing in them are hidden by default — a fifth of a real
  // machine's results, mostly userdata shells Steam creates for every game
  // you own. Kept reachable, since tracking a folder before the game's first
  // save is a legitimate thing to want.
  let showEmpty = false;
  // Which game's folder list is open. One at a time: the panel spans the grid
  // and several open at once turns the tiles into a wall of paths.
  let expandedGroup = null;

  export async function start() {
    scanning = true;
    open = true;
    scanResults = null;
    selected = new Set();
    selectedCount = 0;
    try {
      const results = await api.get('/api/presets/scan');
      fillMissingAppIds(results);
      scanResults = results;
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      scanning = false;
    }
  }

  function close() {
    open = false;
    scanResults = null;
  }

  function onKeydown(e) {
    if (e.key === 'Escape' && open) close();
  }

  // Dismissing a scan result. The exclude list already existed, but only as a
  // folder picker buried in Settings — which meant answering "stop offering
  // me this" required leaving the scan, then finding and re-typing a path you
  // were just looking at. The decision is made here, looking at the result,
  // so it should be actionable here.
  let excluding = null;
  async function excludeResult(item) {
    if (excluding) return;
    const ok = await askConfirm(
      `Stop offering "${item.name}" in future scans? Nothing on disk is touched — this only tells the scanner to skip ${item.savePath}. You can undo it under Settings → Excluded folders.`,
      { title: 'Exclude from scans?', confirmText: 'Exclude' }
    );
    if (!ok) return;
    excluding = item.id;
    try {
      // Read-modify-write against current settings rather than a stored copy:
      // the scan overlay can be open for a while, and clobbering a change
      // made elsewhere in the meantime would be a silent settings loss.
      const current = await api.get('/api/settings');
      const paths = current.excludePaths ?? [];
      if (!paths.includes(item.savePath)) {
        // The store has to take the result, not just the server. The Settings
        // view builds its form by cloning these settings and saves the whole
        // object back, so leaving a stale copy here means the next save from
        // that form writes the old exclusion list over this one — and the file
        // this was meant to keep out of sync quietly starts syncing again.
        settings.set(await api.post('/api/settings', { excludePaths: [...paths, item.savePath] }));
      }
      scanResults = (scanResults ?? []).filter((r) => r.id !== item.id);
      selected.delete(item.id);
      selected = selected;
      toast(`"${item.name}" won't be offered again`, 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      excluding = null;
    }
  }

  $: trackedPaths = new Set($gameList.map((g) => normPath(g.savePath)));
  const isTracked = (r) => trackedPaths.has(normPath(r.savePath));
  // Note: reference trackedPaths directly (not via isTracked) so Svelte sees
  // it as a dependency and refreshes the list when tracked-state changes.

  // Off means no empty folders. It said that before and did not do it: a game
  // whose folders are ALL empty was kept anyway, so the toggle hid some empty
  // tiles and left others sitting there. Keeping a title visible is worth less
  // than a control that behaves, and nothing is lost — the count beside the
  // toggle says how many are hidden and one click brings them back. Empty
  // folders are out of the pool entirely unless asked for, so the tab counts
  // match what the grid shows rather than counting rows nobody can see.
  $: scanPool = showEmpty ? (scanResults ?? []) : (scanResults ?? []).filter((r) => !isEmptyResult(r));
  $: emptyCount = (scanResults ?? []).filter(isEmptyResult).length;

  $: filteredResults = scanPool.filter((r) => {
    if (!showTracked && trackedPaths.has(normPath(r.savePath))) return false;
    if (scanType !== 'all' && r.type !== scanType) return false;
    if (scanFilter && !`${r.name} ${r.savePath}`.toLowerCase().includes(scanFilter.toLowerCase())) return false;
    return true;
  });
  // Keep the saves you can actually add at the top and push already-tracked
  // ones below a divider — with "Show tracked" on, interleaving them buries
  // the actionable entries in a wall of tiles.
  $: allGroups = buildGroups(filteredResults, trackedPaths);
  $: availableGroups = allGroups.filter((g) => !trackedPaths.has(normPath(g.primary.savePath)));
  $: trackedGroups = allGroups.filter((g) => trackedPaths.has(normPath(g.primary.savePath)));
  $: orderedGroups = [...availableGroups, ...trackedGroups];
  $: scanCounts = {
    all: scanPool.length,
    emulator: scanPool.filter((r) => r.type === 'emulator').length,
    repack: scanPool.filter((r) => r.type === 'repack').length,
    game: scanPool.filter((r) => r.type === 'game').length
  };

  function toggleSelect(id) {
    if (selected.has(id)) selected.delete(id);
    else selected.add(id);
    selected = selected; // trigger reactivity
    selectedCount = selected.size;
  }
  // Clicking a game picks the folders the daemon suggests for it — the save
  // folder plus anything sitting beside it that belongs to the same save.
  // Folders it flagged as already covered, or as another install's leftovers,
  // are left for you to tick yourself.
  function toggleGroup(group) {
    const on = group.suggested.every((m) => selected.has(m.id));
    for (const m of group.suggested) {
      if (on) selected.delete(m.id);
      else selected.add(m.id);
    }
    selected = selected;
    selectedCount = selected.size;
  }
  const groupSelected = (group, sel) => group.members.some((m) => sel.has(m.id));
  function selectAllVisible() {
    for (const g of availableGroups) for (const m of g.suggested) selected.add(m.id);
    selected = selected;
    selectedCount = selected.size;
  }
  function clearSelection() {
    selected = new Set();
    selectedCount = 0;
  }

  // Track one game from a set of folders: the first is the save folder and the
  // rest become its extra locations. Adding a location can legitimately fail —
  // the daemon refuses one that sits inside the save folder, because two
  // locations over the same files fight over them — so those are collected and
  // reported rather than swallowed.
  async function trackAsOneGame(primary, extras) {
    const created = await api.post('/api/games', {
      name: primary.name,
      savePath: primary.savePath,
      appId: primary.appId ?? ''
    });
    const gameId = created?.id;
    const failed = [];
    if (gameId) {
      const taken = new Set();
      for (const e of extras) {
        try {
          await api.post(`/api/games/${gameId}/roots`, {
            name: rootNameFor(e.savePath, taken),
            path: e.savePath
          });
        } catch (err) {
          failed.push(`${e.savePath}: ${err.message}`);
        }
      }
    }
    return { gameId, failed };
  }

  // Drop the rows just tracked. With "Show tracked" on they stay and re-render
  // as tracked tiles — removing them would make what you just tracked vanish
  // from a list whose whole point is showing tracked saves.
  function consumeRows(ids) {
    if (scanResults && !showTracked) scanResults = scanResults.filter((r) => !ids.has(r.id));
    for (const id of ids) selected.delete(id);
    selected = selected;
    selectedCount = selected.size;
  }

  async function trackGroup(group) {
    const [primary, ...extras] = group.suggested;
    try {
      const { failed } = await trackAsOneGame(primary, extras);
      consumeRows(new Set(group.suggested.map((m) => m.id)));
      if (failed.length > 0) {
        toast(`Tracking "${primary.name}" — ${failed.length} folder(s) could not be added: ${failed[0]}`, 'error');
      } else if (extras.length > 0) {
        toast(`Now tracking "${primary.name}" across ${extras.length + 1} folders`, 'success');
      } else {
        toast(`Now tracking "${primary.name}"`, 'success');
      }
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function trackSelected() {
    const items = (scanResults ?? []).filter((r) => selected.has(r.id));
    if (items.length === 0) return;

    // Folders of one game go in as one game with locations, not as several
    // games that happen to share a name. That is the whole point of grouping:
    // tracking Scores, Tracks and Profiles separately gives you three library
    // entries that each sync a third of a save.
    let games = 0;
    let locations = 0;
    const problems = [];
    for (const { primary, extras } of plannedGames(items)) {
      try {
        const { failed } = await trackAsOneGame(primary, extras);
        games++;
        locations += extras.length - failed.length;
        problems.push(...failed);
      } catch (e) {
        problems.push(`${primary.name}: ${e.message}`);
      }
    }

    consumeRows(new Set(items.map((i) => i.id)));
    clearSelection();
    const extra = locations > 0 ? ` with ${locations} extra location${locations === 1 ? '' : 's'}` : '';
    toast(`Tracked ${games} game${games === 1 ? '' : 's'}${extra}`, problems.length > 0 ? 'error' : 'success');
    if (problems.length > 0) toast(problems[0], 'error');
    if (filteredResults.length === 0) close();
  }

  // The escape hatch: fold whatever is ticked into one game, whatever the
  // daemon decided. It gets the grouping right most of the time and cannot get
  // it right always — two folders of one game with unrelated names and no
  // AppID have nothing to match on.
  async function mergeSelectedIntoOneGame() {
    const items = (scanResults ?? []).filter((r) => selected.has(r.id));
    if (items.length < 2) return;
    // The most recently written folder leads, matching how the daemon picks a
    // group's primary — the freshest is the save actually being played.
    const ordered = [...items].sort((a, b) => (b.latestMtime ?? 0) - (a.latestMtime ?? 0));
    const [primary, ...extras] = ordered;
    try {
      const { failed } = await trackAsOneGame(primary, extras);
      consumeRows(new Set(items.map((i) => i.id)));
      clearSelection();
      if (failed.length > 0) toast(`Tracked "${primary.name}", but ${failed.length} folder(s) failed: ${failed[0]}`, 'error');
      else toast(`Tracking "${primary.name}" across ${items.length} folders`, 'success');
      if (filteredResults.length === 0) close();
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  const typeLabels = { emulator: 'Emulator', repack: 'Repack', game: 'Game' };
  const typeIcon = (t) => (t === 'emulator' ? Joystick : t === 'repack' ? Package : Gamepad2);
  const typeTabs = [['all', 'All'], ['game', 'Games'], ['emulator', 'Emulators'], ['repack', 'Repacks']];
</script>

<svelte:window on:keydown={onKeydown} />

{#if open}
  <Modal title="Auto-scan results" icon={ScanSearch} onClose={close} width={920} height="min(82vh, 860px)">
    <svelte:fragment slot="sub">
      {#if scanning}Scanning your system…{:else}Found {scanCounts.all} save location{scanCounts.all === 1 ? '' : 's'} — {availableGroups.length} available to track{#if emptyCount > 0 && !showEmpty}, {emptyCount} empty hidden{/if}{/if}
    </svelte:fragment>

    {#if scanning}
      <ModalLoading>Scanning Steam, emulators, and configured folders…</ModalLoading>
    {:else}
      <div class="toolbar">
        <SearchInput placeholder="Filter by name or path…" bind:value={scanFilter} />
        <FilterTabs options={typeTabs} counts={scanCounts} bind:value={scanType} />
        <label class="toggle" title="Also show saves you already track">
          <input type="checkbox" bind:checked={showTracked} />
          Show tracked
        </label>
        {#if emptyCount > 0}
          <label class="toggle" title="Folders that exist but hold no files — Steam makes one for every game you own, whether or not saves go there. A game with nothing saved anywhere is in here too.">
            <input type="checkbox" bind:checked={showEmpty} />
            Show {emptyCount} empty
          </label>
        {/if}
      </div>

      <div class="list">
        <div class="grid">
          {#each orderedGroups as group, i (group.id)}
            {@const item = group.primary}
            {@const tracked = isTracked(item)}
            {#if i === availableGroups.length && trackedGroups.length > 0}
              <div class="divider">
                Already tracked ({trackedGroups.length})
              </div>
            {/if}
            <!--
              Asked for whenever there is anything to ask about. Gating this on an
              App ID skipped exactly the games the name lookup exists to serve: a
              title sold only on GOG or itch has none, so no request was ever made.
              The daemon answers 404 when it finds nothing and the tile falls back
              to the name, so asking costs a cached miss and nothing more.
            -->
            <CoverTile
              name={item.name}
              title={item.savePath}
              src={coverURL(item.appId, true, item.name)}
              icon={typeIcon(item.type)}
              selected={groupSelected(group, selected)}
              badge={typeLabels[item.type] ?? item.type}
              stamp={tracked ? 'Tracked' : ''}
              dimmed={tracked}
              faded={isEmptyResult(item)}
              meta={contentsLabel(item)}
              metaTitle={item.savePath}
              on:activate={() => toggleGroup(group)}
            >
              <svelte:fragment slot="hover">
                <button class="btn small primary" on:click|stopPropagation={() => trackGroup(group)}>
                  {group.suggested.length > 1 ? `Track all ${group.suggested.length}` : 'Track'}
                </button>
                <button
                  class="btn small"
                  disabled={excluding === item.id}
                  title="Stop offering this location in future scans"
                  on:click|stopPropagation={() => excludeResult(item)}
                >
                  {excluding === item.id ? 'Excluding…' : 'Exclude'}
                </button>
              </svelte:fragment>
              {#if group.extras.length > 0}
                <button
                  class="folders"
                  class:open={expandedGroup === group.id}
                  on:click|stopPropagation={() => (expandedGroup = expandedGroup === group.id ? null : group.id)}
                >
                  <Chevron open={expandedGroup === group.id} size={12} /> found in {group.members.length} folders
                  {#if group.suggested.length > 1}<span class="folders-hint">· {group.suggested.length} are one save</span>{/if}
                </button>
              {/if}
            </CoverTile>

            {#if expandedGroup === group.id}
              <ScanFolderList {group} {selected} {trackedPaths} on:toggle={(e) => toggleSelect(e.detail)} />
            {/if}
          {:else}
            <div class="empty-grid">
              {scanCounts.all === 0 ? 'Nothing detected. You can still track any folder manually.' : 'No matches for this filter.'}
            </div>
          {/each}
        </div>
      </div>

      <ModalFoot>
        <div class="actions">
          <button class="btn small" on:click={selectAllVisible} disabled={availableGroups.length === 0}>Select all ({availableGroups.length})</button>
          {#if selectedCount > 0}
            <button class="btn small" on:click={clearSelection}>Clear</button>
          {/if}
        </div>
        <!-- Both tracking actions sit together on the right: they are the
             two answers to the same question — one game or several — and
             splitting them across the bar made the merge read as a filter. -->
        <div class="actions">
          {#if selectedCount > 1}
            <button
              class="btn primary"
              title="Track everything ticked as a single game, whatever it was grouped under. For a split save the scan did not spot."
              on:click={mergeSelectedIntoOneGame}
            >
              Track as one game
            </button>
          {/if}
          <button class="btn primary" disabled={selectedCount === 0} on:click={trackSelected}>
            Track selected ({selectedCount})
          </button>
        </div>
      </ModalFoot>
    {/if}
  </Modal>
{/if}

<style>
  .toolbar {
    display: flex;
    gap: 12px;
    padding: 14px 22px;
    align-items: center;
    flex-wrap: wrap;
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 7px 13px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: transparent;
    font-size: 0.85rem;
    color: var(--text-dim);
    white-space: nowrap;
    cursor: pointer;
  }
  .toggle:hover {
    background: var(--bg-hover);
  }
  /* A notch smaller than the app-wide 17px: this one sits in a compact pill
     next to 0.85rem text, and the full-size circle makes the pill taller
     than the buttons beside it. */
  .toggle input {
    width: 15px;
    height: 15px;
  }
  .list {
    flex: 1;
    overflow-y: auto;
    padding: 4px 22px 8px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 16px;
  }
  /* Full-width heading separating the already-tracked group from the saves
     you can still add. */
  .divider {
    grid-column: 1 / -1;
    display: flex;
    align-items: center;
    gap: 10px;
    margin: 10px 0 2px;
    color: var(--text-faint);
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .divider::after {
    content: '';
    flex: 1;
    height: 1px;
    background: var(--border);
  }
  /* ── A game found in more than one place ───────────────────────── */
  .folders {
    margin-top: 4px;
    width: 100%;
    background: none;
    border: 0;
    padding: 2px 0;
    font-size: 0.72rem;
    color: var(--accent);
    cursor: pointer;
    text-align: center;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .folders:hover,
  .folders.open {
    text-decoration: underline;
  }
  .folders-hint {
    color: var(--text-faint);
  }
  .empty-grid {
    grid-column: 1 / -1;
    text-align: center;
    color: var(--text-faint);
    padding: 50px 20px;
  }
  .actions {
    display: flex;
    gap: 8px;
    align-items: center;
  }
</style>
