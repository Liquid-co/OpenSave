<script>
  import { onDestroy } from 'svelte';
  import { stateLoaded, gameList, peers, syncActivity, conflicts, locationConflicts, toast, syncPause, activityTick } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';
  import { gameStatusParts, conflictedIds } from '../lib/gamestatus.js';
  import { timeAgoParts } from '../lib/timeago.js';
  import { playLength } from '../lib/format.js';
  import OfferedGames from './home/OfferedGames.svelte';
  import AddGameCard from './home/AddGameCard.svelte';
  import TrackGameDialog from './home/TrackGameDialog.svelte';
  import ScanDialog from './home/ScanDialog.svelte';
  import LibraryGrid from './home/LibraryGrid.svelte';
  import LibraryBackdrop from './home/LibraryBackdrop.svelte';
  import RecentlyPlayed from './home/RecentlyPlayed.svelte';
  import NeedsYou from './home/NeedsYou.svelte';
  import SplitButton from '../components/ui/SplitButton.svelte';
  import { recentlyPlayed, backdropGame, playedFromReport } from '../lib/libraryhome.js';
  import { recentHidden } from '../lib/recenthidden.js';
  import { libraryHeadline } from '../lib/headline.js';
  import { libraryView } from '../lib/libraryview.js';
  import { rise, collapse } from '../lib/motion.js';
  import Skeleton from '../components/ui/Skeleton.svelte';
  import { visibleGames, syncAllGames } from '../lib/gameactions.js';
  import SetupGuide from './home/SetupGuide.svelte';
  import { setupState, setupSteps, decideSetupFor } from '../lib/setup.js';
  import { settings } from '../lib/stores.js';
  import { t, segments } from '../lib/i18n.js';
  import ScanSearch from 'lucide-svelte/icons/scan-search';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import FolderPlus from 'lucide-svelte/icons/folder-plus';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Plus from 'lucide-svelte/icons/plus';
  import SquareCheckBig from 'lucide-svelte/icons/square-check-big';

  export let params = {};

  let scanner;
  let scanning = false;

  // Tracking by hand: "Track folder" goes straight to the system's folder
  // picker, and the folder picked is then named (AddGameCard). A folder
  // dropped on the window arrives with its path and skips the picker. Keyed
  // on the params object, so each request — the sidebar's +, Ctrl+K — opens
  // it once, and not again on every re-render.
  let showAdd = false;
  // Tracking a game the game database knows (TrackGameDialog).
  let showTrackGame = false;
  let addPath = '';
  let handledAdd = null;
  $: if (params.add && params !== handledAdd) {
    handledAdd = params;
    if (params.path) {
      addPath = params.path;
      showAdd = true;
    } else {
      trackFolder();
    }
  }

  async function trackFolder() {
    if (!native.isWails()) {
      // No picker in a browser: the dialog takes a typed path instead.
      addPath = '';
      showAdd = true;
      return;
    }
    const dir = await native.selectDirectory($t('home.chooseFolderDialog'));
    if (dir) {
      addPath = dir;
      showAdd = true;
    }
  }

  // Opened from the new-games card. A token rather than a flag: this
  // statement re-runs whenever the scan closes, and a flag still set would
  // start the scan all over again. Waits for the dialog to exist, which it
  // does not on the first pass.
  let handledScan = null;
  $: if (scanner && params.scan && params.scan !== handledScan) {
    handledScan = params.scan;
    scanner.start();
  }

  function syncAll() {
    syncAllGames();
    toast($t('home.syncAllToast'));
  }

  // The Library's two buttons. Each does the usual thing; its arrow offers
  // the rest. Choosing games to sync is the grid's own selecting, which has
  // Sync among what it does to the games picked.
  let grid;
  $: addItems = [
    { label: $t('home.scanSaves'), icon: ScanSearch, run: () => scanner.start(), disabled: scanning, hint: $t('home.add.scanHint') },
    { label: $t('home.trackGame'), icon: Gamepad2, run: () => (showTrackGame = true), hint: $t('home.add.trackGameHint') },
    { label: $t('home.trackFolder'), icon: FolderPlus, run: trackFolder, hint: $t('home.add.trackFolderHint') }
  ];
  $: syncItems = [
    { label: $t('home.sync.all'), icon: RefreshCw, run: syncAll, disabled: $gameList.length === 0 || $syncPause.paused },
    { label: $t('home.sync.choose'), icon: SquareCheckBig, run: () => grid?.chooseToSync(), disabled: $gameList.length === 0 }
  ];

  // Who played what, and where, from the activity report: it knows another
  // device's play from its save arriving here. Asked again as things happen.
  let played = {};
  async function loadPlayed() {
    try {
      played = playedFromReport(await api.get('/api/activity?limit=1'));
    } catch {
      // The tiles fall back on this device's own record.
    }
  }
  $: $activityTick, loadPlayed();
  // More than fit across: the row turns into a belt to drag through.
  $: recent = recentlyPlayed($visibleGames, played, 12, $recentHidden);
  $: lit = backdropGame($visibleGames, played, $recentHidden);
  $: headline = libraryHeadline(rows);

  // Where each game's save stands — the summary and the cards read the same
  // answer, so the two can never disagree. "4 min ago" must not freeze at
  // the moment the page was opened.
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));
  $: conflicted = conflictedIds($conflicts, $locationConflicts);

  // timeAgo's facts as the catalog words for it.
  function agoLine(at, now, say) {
    const p = timeAgoParts(at, now);
    if (p.kind === 'invalid') return p.text;
    const { kind, ...params } = p;
    return say('timeAgo.' + kind, params);
  }
  // Each status line's wording is built in lib/gamestatus.js, in English;
  // the same states map here to catalog keys (see locales/en.json,
  // "gameStatus."), with the time the line is about — gameStatusParts'
  // `when` — said the catalog's way.
  function statusLine(parts, now, say) {
    const when = parts.when ? agoLine(parts.when, now, say) : '';
    switch (parts.state) {
      case 'elsewhere': return say('gameStatus.elsewhere');
      case 'missing': return say('gameStatus.missing');
      case 'emptied': return say('gameStatus.emptied');
      case 'restoring': return say('gameStatus.restoring');
      case 'conflict': return say('gameStatus.conflict');
      case 'playing': {
        const ms = parts.when == null ? NaN : now - Date.parse(parts.when);
        return Number.isFinite(ms) && ms >= 60_000 ? say('gameStatus.inSessionFor', { length: playLength(ms) }) : say('gameStatus.inSession');
      }
      case 'syncing': return say('gameStatus.syncing', { pct: parts.pct ?? 0 });
      case 'error': return say('gameStatus.error');
      case 'paused': return say('gameStatus.paused');
      case 'synced': return say('gameStatus.synced', { when });
      case 'unsynced': return say('gameStatus.unsynced');
      case 'local': return say('gameStatus.local', { when });
      case 'empty': return say('gameStatus.empty');
      default: return parts.label;
    }
  }
  $: rows = $visibleGames.map((game) => {
    const parts = gameStatusParts(game, {
      peers: $peers,
      activity: $syncActivity[game.id],
      conflicted: conflicted.has(game.id),
      now
    });
    return { game, status: { ...parts, label: statusLine(parts, now, $t) } };
  });
  // The setup guide, until everything in it is done or skipped, or it is put
  // away. Games are counted as tracked, not as shown, so a game on its way out
  // does not bring the guide back for a moment.
  $: if ($stateLoaded && !$setupState.seen) setupState.set(decideSetupFor($setupState, $gameList.length));
  $: showGuide =
    $setupState.seen &&
    !$setupState.dismissed &&
    !setupSteps({ games: $gameList.length, peers: Object.keys($peers).length, cloud: $settings?.cloudSync?.ready, skipped: $setupState.skipped }).finished;
</script>

<div class="library">
<LibraryBackdrop game={lit} games={$visibleGames} />

<div class="head">
  <div class="title-block">
    <h1 class="page-title">{$t('home.title')}</h1>
    {#if $stateLoaded && $visibleGames.length > 0}
      <p class="sub" role="status">{#key headline.key + JSON.stringify(headline.params)}<span class="swap" in:rise={{ y: 4, duration: 200 }}>{$t(headline.key, headline.params)}</span>{/key}</p>
    {/if}
  </div>
  <div class="head-actions">
    <SplitButton
      label={$t('home.sync.button')}
      icon={RefreshCw}
      disabled={$gameList.length === 0 || $syncPause.paused}
      title={$syncPause.paused ? $t('home.syncPaused') : $t('home.sync.allTitle')}
      items={syncItems}
      menuLabel={$t('home.sync.more')}
      on:click={syncAll}
    />
    <SplitButton
      primary
      label={$t('home.add.button')}
      icon={Plus}
      title={$t('home.scanSavesTitle')}
      items={addItems}
      menuLabel={$t('home.add.more')}
      on:click={() => scanner.start()}
    />
  </div>
</div>
<OfferedGames />

{#if showAdd}
  <AddGameCard path={addPath} on:close={() => (showAdd = false)} />
{/if}

{#if showTrackGame}
  <TrackGameDialog on:close={() => (showTrackGame = false)} />
{/if}

<ScanDialog bind:this={scanner} bind:scanning />

{#if !$stateLoaded}
  <Skeleton kind="tiles" count={6} />
{:else if showGuide && $visibleGames.length === 0}
  <SetupGuide {scanning} on:scan={() => scanner.start()} on:add={trackFolder} />
{:else if $visibleGames.length === 0}
  <div class="welcome">
    <div class="welcome-icon"><Gamepad2 size={34} strokeWidth={1.6} /></div>
    <h3>{$t('home.welcome.title')}</h3>
    <p>{$t('home.welcome.intro')}</p>
    <div class="welcome-actions">
      <button class="btn primary" on:click={() => scanner.start()} disabled={scanning}>
        <ScanSearch size={16} />{scanning ? $t('home.scanning') : $t('home.welcome.scanForSaves')}
      </button>
      <button class="btn" on:click={trackFolder}><FolderPlus size={16} />{$t('home.welcome.trackManually')}</button>
    </div>
    <p class="welcome-hint">
      {#each segments($t('home.welcome.hint'), { devices: $t('home.devices'), cloud: $t('home.cloudBackup') }, ['devices', 'cloud']) as part}
        {#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}
      {/each}
    </p>
  </div>
{:else}
  {#if showGuide}
    <SetupGuide {scanning} on:scan={() => scanner.start()} on:add={trackFolder} />
  {/if}
  <div class="top">
    <NeedsYou />
    {#if $libraryView.overview && recent.length}
      <div transition:collapse><RecentlyPlayed items={recent} /></div>
    {/if}
  </div>
  <LibraryGrid {rows} bind:this={grid} />
{/if}
</div>

<style>
  /* Its own stacking context, so the backdrop (z-index -1) sits behind the
     page's content and nowhere else. */
  .library {
    position: relative;
    isolation: isolate;
  }
  .top {
    display: flex;
    flex-direction: column;
    gap: 20px;
    margin-bottom: 26px;
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 20px;
    gap: 12px 16px;
    flex-wrap: wrap;
  }
  .page-title {
    font-size: var(--text-2xl);
    font-weight: 600;
    letter-spacing: -0.01em;
  }
  .swap {
    display: inline-block;
  }
  .sub {
    margin-top: 2px;
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .head-actions {
    display: flex;
    gap: 10px;
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
    border-radius: calc(18px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent-text);
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
