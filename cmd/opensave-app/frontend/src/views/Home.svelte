<script>
  import { onDestroy } from 'svelte';
  import { stateLoaded, gameList, peers, syncActivity, conflicts, locationConflicts, toast, syncPause } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import { gameStatus, conflictedIds } from '../lib/gamestatus.js';
  import OfferedGames from './home/OfferedGames.svelte';
  import AddGameCard from './home/AddGameCard.svelte';
  import ScanDialog from './home/ScanDialog.svelte';
  import HomeSummary from './home/HomeSummary.svelte';
  import LibraryGrid from './home/LibraryGrid.svelte';
  import Skeleton from '../components/ui/Skeleton.svelte';
  import { visibleGames } from '../lib/gameactions.js';
  import SetupGuide from './home/SetupGuide.svelte';
  import PauseButton from '../components/PauseButton.svelte';
  import { setupState, setupSteps, decideSetupFor } from '../lib/setup.js';
  import { settings } from '../lib/stores.js';
  import ScanSearch from 'lucide-svelte/icons/scan-search';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import FolderPlus from 'lucide-svelte/icons/folder-plus';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';

  export let params = {};

  let scanner;
  let scanning = false;

  let showAdd = params.add ?? false;
  $: if (params.add) showAdd = true;
  // Opened from the new-games card. A token rather than a flag: this
  // statement re-runs whenever the scan closes, and a flag still set would
  // start the scan all over again. Waits for the dialog to exist, which it
  // does not on the first pass.
  let handledScan = null;
  $: if (scanner && params.scan && params.scan !== handledScan) {
    handledScan = params.scan;
    scanner.start();
  }

  async function syncAll() {
    for (const g of $gameList) {
      api.post(`/api/games/${g.id}/sync`).catch(() => {});
    }
    toast('Sync triggered for all games');
  }

  // Where each game's save stands — the summary and the cards read the same
  // answer, so the two can never disagree. "4 min ago" must not freeze at
  // the moment the page was opened.
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));
  $: conflicted = conflictedIds($conflicts, $locationConflicts);
  // The setup guide, until everything in it is done or skipped, or it is put
  // away. Games are counted as tracked, not as shown, so a game on its way out
  // does not bring the guide back for a moment.
  $: if ($stateLoaded && !$setupState.seen) setupState.set(decideSetupFor($setupState, $gameList.length));
  $: showGuide =
    $setupState.seen &&
    !$setupState.dismissed &&
    !setupSteps({ games: $gameList.length, peers: Object.keys($peers).length, cloud: $settings?.cloudSync?.enabled, skipped: $setupState.skipped }).finished;

  $: rows = $visibleGames.map((game) => ({
    game,
    status: gameStatus(game, {
      peers: $peers,
      activity: $syncActivity[game.id],
      conflicted: conflicted.has(game.id),
      now
    })
  }));
</script>

<div class="head">
  <h2 class="page-title">Home</h2>
  <div class="head-actions">
    <button class="btn" on:click={() => scanner.start()} disabled={scanning}>
      <ScanSearch size={16} />{scanning ? 'Scanning…' : 'Auto-scan'}
    </button>
    <button class="btn" on:click={syncAll} disabled={$gameList.length === 0 || $syncPause.paused} title={$syncPause.paused ? 'Syncing is paused' : ''}><RefreshCw size={15} />Sync all</button>
    <PauseButton />
    <button class="btn primary" on:click={() => (showAdd = !showAdd)}><FolderPlus size={16} />Track folder</button>
  </div>
</div>

<OfferedGames />

{#if showAdd}
  <AddGameCard initialPath={params.path ?? ''} on:close={() => (showAdd = false)} />
{/if}

<ScanDialog bind:this={scanner} bind:scanning />

{#if !$stateLoaded}
  <Skeleton kind="tiles" count={6} />
{:else if showGuide && $visibleGames.length === 0}
  <SetupGuide {scanning} on:scan={() => scanner.start()} on:add={() => (showAdd = true)} />
{:else if $visibleGames.length === 0}
  <div class="welcome">
    <div class="welcome-icon"><Gamepad2 size={34} strokeWidth={1.6} /></div>
    <h3>Welcome to OpenSave</h3>
    <p>Keep your game saves in sync across every device — no accounts, no cloud lock-in. Start by finding your saves:</p>
    <div class="welcome-actions">
      <button class="btn primary" on:click={() => scanner.start()} disabled={scanning}>
        <ScanSearch size={16} />{scanning ? 'Scanning…' : 'Auto-scan for saves'}
      </button>
      <button class="btn" on:click={() => (showAdd = true)}><FolderPlus size={16} />Track a folder manually</button>
    </div>
    <p class="welcome-hint">Then open <strong>Devices</strong> to pair another PC or Steam Deck, or <strong>Cloud Backup</strong> to mirror snapshots online.</p>
  </div>
{:else}
  {#if showGuide}
    <SetupGuide {scanning} on:scan={() => scanner.start()} on:add={() => (showAdd = true)} />
  {/if}
  <div class="top">
    <HomeSummary {rows} />
  </div>
  <LibraryGrid {rows} />
{/if}

<style>
  .top {
    display: flex;
    flex-direction: column;
    gap: 14px;
    margin-bottom: 26px;
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 20px;
    gap: 12px;
    flex-wrap: wrap;
  }
  .head-actions {
    display: flex;
    gap: 8px;
  }

  .welcome {
    text-align: center;
    max-width: 520px;
    margin: 40px auto;
    padding: 40px 28px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }
  .welcome-icon {
    width: 64px;
    height: 64px;
    margin: 0 auto 14px;
    border-radius: 18px;
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent);
  }
  .welcome h3 {
    font-size: 1.3rem;
    font-weight: 700;
    margin-bottom: 10px;
  }
  .welcome p {
    color: var(--text-dim);
    font-size: 0.92rem;
    line-height: 1.55;
  }
  .welcome-actions {
    display: flex;
    gap: 10px;
    justify-content: center;
    margin: 22px 0 6px;
    flex-wrap: wrap;
  }
  .welcome-hint {
    font-size: 0.8rem;
    color: var(--text-faint);
    margin-top: 16px;
  }
</style>
