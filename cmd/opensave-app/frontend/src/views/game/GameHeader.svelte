<script>
  // The top of a game's page: its art and name, which devices have this save,
  // the Launch and Sync buttons, and the save folder with a way to open it.
  import { onDestroy } from 'svelte';
  import { peers, navigate, toast, syncActivity, askConfirm } from '../../lib/stores.js';
  import { emptiedWhere } from '../../lib/emptied.js';
  import { whenLabel } from '../../lib/snapshots.js';
  import RotateCcw from 'lucide-svelte/icons/rotate-ccw';
  import { api, native, gameCover } from '../../lib/api.js';
  import { timeAgo } from '../../lib/timeago.js';
  import { playLength } from '../../lib/format.js';
  import { canLaunch, cleanGameName, renameGame, MAX_GAME_NAME } from '../../lib/gameactions.js';
  import Spinner from '../../components/ui/Spinner.svelte';
  import { restoreProgressText } from '../../lib/restoreprogress.js';
  import { t as translateUI } from '../../lib/i18n.js';
  import ArrowLeft from 'lucide-svelte/icons/arrow-left';
  import Play from 'lucide-svelte/icons/play';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import Pencil from 'lucide-svelte/icons/pencil';
  import Star from 'lucide-svelte/icons/star';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import Info from 'lucide-svelte/icons/info';
  import { collections, isFavourite, toggleFavourite } from '../../lib/collections.js';

  export let game;
  /** The page's shared runner (see lib/runner.js). */
  export let runner;
  /** Set (to a new value each time) when "Rename…" was chosen in a menu or
   *  Ctrl+K: the name opens for editing. */
  export let renameRequest = undefined;
  const { busy, run } = runner;

  // Renaming, in place: the name becomes a field. Enter keeps it, Esc puts the
  // old one back. The ID stays, so nothing about syncing changes.
  let editName = false;
  let nameDraft = '';
  let nameSaving = false;
  $: nameProblem = editName ? cleanGameName(nameDraft).error : undefined;
  $: if (renameRequest) startRename();

  function startRename() {
    nameDraft = game.name;
    editName = true;
  }
  async function saveName() {
    if (nameProblem || nameSaving) return;
    nameSaving = true;
    const ok = await renameGame(game, nameDraft);
    nameSaving = false;
    if (ok) editName = false;
  }
  function nameKeys(e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      saveName();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation(); // Esc here cancels the edit, not the page
      editName = false;
    }
  }
  function focusSelect(el) {
    el.focus();
    el.select();
  }

  $: activity = $syncActivity[game.id];

  // Is my Deck up to date with THIS save? One entry per paired device, in
  // the header, because it is the question a person opens a game to ask.
  // "never" is listed too: it is the answer that explains why a save is not
  // on the other machine. A device that was unpaired is not shown even if a
  // stamp for it survived somewhere — the list is the paired devices.
  $: syncedWith = Object.values($peers)
    .map((p) => ({ id: p.id, name: p.name, at: game?.lastSyncedWith?.[p.id] ?? '' }))
    .sort((a, b) => a.name.localeCompare(b.name));
  // "4 min ago" must not freeze at the moment the page was opened.
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));

  const syncNow = () => run($translateUI('gameHeader.syncTriggered'), () => api.post(`/api/games/${game.id}/sync`));
  const launchGame = () => run($translateUI('gameHeader.launching'), () => api.post(`/api/games/${game.id}/launch`));

  let editPath = false;
  let pathDraft = '';

  // The answer to an emptied save: put the files back here, or let the
  // deletion go to the other devices too.
  let answering = false;
  async function answerEmptied(answer) {
    if (answer === 'delete') {
      const n = game.emptied.files;
      const ok = await askConfirm(
        $translateUI('gameHeader.deleteSSaveFilesOnYourOther', { p0: game.name, p1: n, p2: n === 1 ? '' : 's' }),
        { title: $translateUI('gameHeader.deleteThemEverywhere'), confirmText: $translateUI('gameHeader.deleteThemThereToo'), danger: true }
      );
      if (!ok) return;
    }
    answering = true;
    try {
      const res = await api.post(`/api/games/${game.id}/emptied`, { answer });
      if (answer === 'delete') toast($translateUI('gameHeader.sSaveFilesAreDeletedOnYour', { p0: game.name }), 'success');
      else if (res.fetching > 0) toast($translateUI('gameHeader.puttingSFilesBackComeFrom', { p0: game.name, p1: res.fetching }), 'success');
      else toast($translateUI('gameHeader.sFilesAreBack', { p0: game.name }), 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      answering = false;
    }
  }
  // The save folder is missing here, and the same folder exists on another drive.
  // It is offered, never switched to on its own: it could be an old copy.
  let relocating = false;
  async function answerRelocation(use) {
    const found = game.relocateSuggestion;
    relocating = true;
    try {
      if (use) {
        await api.post(`/api/games/${game.id}/relocate`, { path: found });
        toast($translateUI('gameHeader.nowUses', { p0: game.name, p1: found }), 'success');
      } else {
        await api.post(`/api/games/${game.id}/relocate/dismiss`);
        toast($translateUI('gameHeader.notUsingFor', { p0: found, p1: game.name }), 'success');
      }
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      relocating = false;
    }
  }
  // Reveal the save location in Explorer / Finder / the Linux file manager.
  // The bridge returns a message when it can't (e.g. the folder was deleted).
  async function openSaveFolder() {
    const problem = await native.openFolder(game.savePath);
    if (problem) toast(problem, 'error');
  }

  async function savePath() {
    await run($translateUI('gameHeader.savePathUpdated'), () => api.patch(`/api/games/${game.id}`, { savePath: pathDraft }));
    editPath = false;
  }
</script>

<div class="head">
  <button class="btn icon back" on:click={() => navigate('home')} title="{$translateUI('cloudBrowser.back')}" aria-label="{$translateUI('cloudBrowser.back')}"><ArrowLeft size={18} /></button>
  {#if gameCover(game)}
    <img
      class="head-cover"
      src={gameCover(game)}
      alt=""
      on:load={(e) => (e.currentTarget.style.display = '')}
      on:error={(e) => (e.currentTarget.style.display = 'none')}
    />
  {/if}
  <div class="title-block">
    <h2 class="page-title title-row">
      {#if editName}
        <input
          class="name-input"
          class:invalid={!!nameProblem}
          bind:value={nameDraft}
          maxlength={MAX_GAME_NAME + 20}
          aria-label="{$translateUI('gameHeader.gameName')}"
          aria-invalid={!!nameProblem}
          use:focusSelect
          on:keydown={nameKeys}
        />
        <button class="btn small primary" disabled={!!nameProblem || nameSaving} on:click={saveName}>{$translateUI('gameHeader.save')}</button>
        <button class="btn small" on:click={() => (editName = false)}>{$translateUI('common.cancel')}</button>
      {:else}
        {game.name}
        <button
          class="btn small ghost icon rename"
          title="{$translateUI('gameHeader.renameOnlyOnThisDeviceSyncingIs')}"
          aria-label="{$translateUI('common.rename')}"
          on:click={startRename}
        >
          <Pencil size={14} />
        </button>
      {/if}
      <button
        class="btn small ghost icon star"
        class:on={isFavourite($collections, game.id)}
        title={isFavourite($collections, game.id) ? $translateUI('gameHeader.removeFromFavourites') : $translateUI('gameHeader.addToFavourites')}
        aria-label={isFavourite($collections, game.id) ? $translateUI('gameHeader.removeFromFavourites') : $translateUI('gameHeader.addToFavourites')}
        aria-pressed={isFavourite($collections, game.id)}
        on:click={() => toggleFavourite($collections, game)}
      >
        <Star size={16} />
      </button>
    </h2>
    {#if editName}
      <div class="sub name-hint" class:problem={!!nameProblem} role={nameProblem ? 'alert' : undefined}>
        {nameProblem ?? $translateUI('gameHeader.onlyOnThisDeviceYourOtherDevices')}
      </div>
    {/if}
    <div class="sub">
      {$translateUI('gameHeader.branch')} <strong>{game.activeBranch}</strong>
      {#if activity?.state === 'running'}
        · <span class="syncing">{$translateUI('gameHeader.syncing')} {activity.percentage ?? 0}%</span>
      {/if}
      {#if game.restoring}
        · <span class="syncing"><Spinner size={11} /> {restoreProgressText(game.restoring, $translateUI)}</span>
      {/if}
      {#if game.playingSince}
        · <span class="playing">{$translateUI('gameHeader.inSessionSince')} {new Date(game.playingSince).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
      {:else if game.lastPlayedAt}
        · <span title={new Date(game.lastPlayedAt).toLocaleString()}>{$translateUI('gameHeader.played')} {timeAgo(game.lastPlayedAt, now)}, {playLength(game.playtimeMs)} {$translateUI('gameHeader.inAll')}</span>
      {/if}
      {#if game.installed === 'not-found'}
        · <span title="{$translateUI('gameHeader.steamOnThisPCDoesNotHave')}">{$translateUI('gameHeader.notFoundOnThisPC')}</span>
      {/if}
    </div>
    {#if syncedWith.length > 0}
      <div class="sub synced-with">
        {#each syncedWith as d, i (d.id)}
          {#if i > 0}<span class="sep">·</span>{/if}
          <span class="device" title={d.at ? $translateUI('gameHeader.lastSynced', { p0: d.name, p1: new Date(d.at).toLocaleString() }) : $translateUI('gameHeader.hasNeverSyncedThisGame', { p0: d.name })}>
            {d.name}
            <span class:never={!d.at}>{d.at ? $translateUI('gameHeader.synced', { p0: timeAgo(d.at, now) }) : $translateUI('gameHeader.neverSynced')}</span>
          </span>
        {/each}
      </div>
    {/if}
  </div>
  <div class="head-actions">
    {#if canLaunch(game)}
      <button class="btn" disabled={$busy} on:click={launchGame}><Play size={15} />{$translateUI('gameHeader.launch')}</button>
    {/if}
    <button class="btn primary" disabled={$busy} on:click={syncNow}><RefreshCw size={15} />{$translateUI('gameHeader.syncNow')}</button>
  </div>
</div>

<div class="path-line">
  {#if editPath}
    <input class="path-input" bind:value={pathDraft} />
    <button class="btn small" on:click={async () => (pathDraft = (await native.selectDirectory($translateUI('gameHeader.selectSaveFolder'))) || pathDraft)}>{$translateUI('providerFields.browse')}</button>
    <button class="btn small primary" on:click={savePath}>{$translateUI('gameHeader.save')}</button>
    <button class="btn small" on:click={() => (editPath = false)}>{$translateUI('common.cancel')}</button>
  {:else}
    <span class="path" title={game.savePath}>{game.savePath}</span>
    <button class="btn small" on:click={openSaveFolder} title="{$translateUI('gameHeader.showThisFolderInYourFileManager')}">
      <FolderOpen size={14} />{$translateUI('gameHeader.openFolder')}
    </button>
    <button class="btn small" on:click={() => { pathDraft = game.savePath; editPath = true; }}><Pencil size={13} />{$translateUI('gameHeader.edit')}</button>
  {/if}
</div>

{#if game.emptied && !game.savePathMissing}
  <!-- Every save file went at once here. Nothing is synced until this is
       answered: syncing it would delete them on the other devices too
       (internal/p2p/syncengine/hold.go). -->
  <div class="missing emptied" role="status">
    <TriangleAlert size={16} />
    <div>
      {#if game.emptied.state === 'fetching'}
        <strong>{$translateUI('gameHeader.puttingTheFilesBack')}</strong>
        {$translateUI('gameHeader.someAreStillToComeFromYour')}
      {:else}
        <strong>{$translateUI('gameHeader.everySaveFileIn')} {emptiedWhere(game.emptied.locations)} {$translateUI('gameHeader.wasDeletedOnThisDevice')}</strong>
        {$translateUI('gameHeader.yourOtherDevicesStillHaveTheirsAnd')}
        {#if game.emptied.putBackFrom}
          {$translateUI('gameHeader.putThemBackFromTheSnapshotOf')} {whenLabel(game.emptied.putBackFromTime)}{$translateUI('gameHeader.withAnythingNewerFromYourOther')}
        {:else}
          {$translateUI('gameHeader.putThemBackFromYourOtherDevices')}
        {/if}
        {$translateUI('gameHeader.orIfYouMeantItDelete')}
        <div class="emptied-actions">
          <button class="btn small primary" disabled={answering} on:click={() => answerEmptied('restore')}>
            <RotateCcw size={13} />{$translateUI('gameHeader.putThemBack')}
          </button>
          <button class="btn small" disabled={answering} on:click={() => answerEmptied('delete')}>
            {$translateUI('gameHeader.deleteThemOnMyOtherDevicesToo')}
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}

{#if game.savePathMissing && game.saveDriveMissing && !editPath}
  <!-- The drive itself isn't on this device: the game lives on another PC's
       D:\ or E:\. Nothing was lost here, so this informs rather than warns. -->
  <div class="missing elsewhere" role="status">
    <Info size={16} />
    <div>
      <strong>{$translateUI('gameHeader.thisGameIsnTOnThisDevice')}</strong>
      {$translateUI('gameHeader.itsSavesAreOn')} {game.savePath.slice(0, 2)}{$translateUI('gameHeader.whichThisPCDoesnTHaveSo')}
    </div>
  </div>
{:else if game.savePathMissing && !editPath}
  <!-- The folder is not there. It is not created again: an empty folder in
       its place would read as every file deleted, and syncing that would
       delete them on your other devices too. -->
  <div class="missing" role="status">
    <TriangleAlert size={16} />
    <div>
      <strong>{$translateUI('gameHeader.thisSaveFolderIsNotThere')}</strong>
      {$translateUI('gameHeader.itMayHaveBeenMovedOrDeleted')}
    </div>
  </div>
{/if}

{#if game.savePathMissing && game.relocateSuggestion && !editPath}
  <!-- Offered, not taken: the same path on another drive can be an old
       backup, and syncing from it would spread stale saves. -->
  <div class="missing relocate" role="status">
    <Info size={16} />
    <div>
      <strong>{$translateUI('gameHeader.theSameFolderIsAt')} {game.relocateSuggestion}.</strong>
      {$translateUI('gameHeader.isThatWhereThisGameSavesOn')}
      <div class="relocate-actions">
        <button class="btn small primary" disabled={relocating} on:click={() => answerRelocation(true)}>{$translateUI('gameHeader.useThisFolder')}</button>
        <button class="btn small ghost" disabled={relocating} on:click={() => answerRelocation(false)}>{$translateUI('gameHeader.notThisOne')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .title-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .star,
  .rename {
    color: var(--text-faint);
  }
  .name-input {
    font: inherit;
    min-width: 0;
    flex: 1;
    max-width: 32em;
    padding: 2px 8px;
  }
  .name-input.invalid {
    border-color: var(--danger);
  }
  .name-hint.problem {
    color: var(--danger);
  }
  .star.on {
    color: var(--warn);
  }
  .star.on :global(svg) {
    fill: currentColor;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-bottom: 6px;
  }
  .back {
    font-size: 1rem;
  }
  .head-cover {
    height: 52px;
    aspect-ratio: 460 / 215;
    object-fit: cover;
    border-radius: 8px;
    border: 1px solid var(--border);
  }
  .title-block {
    flex: 1;
    min-width: 0;
  }
  .sub {
    color: var(--text-dim);
    font-size: 0.85rem;
    margin-top: 2px;
  }
  .syncing {
    color: var(--accent-text);
    font-weight: 600;
  }
  .synced-with {
    display: flex;
    flex-wrap: wrap;
    gap: 0 6px;
  }
  .synced-with .device {
    white-space: nowrap;
  }
  .synced-with .device > span {
    color: var(--text);
  }
  .synced-with .device > span.never {
    color: var(--text-dim);
    font-style: italic;
  }
  .synced-with .sep {
    color: var(--text-dim);
  }
  .head-actions {
    display: flex;
    gap: 8px;
  }
  .missing {
    display: flex;
    gap: 10px;
    margin: -8px 0 18px 50px;
    padding: 10px 12px;
    border: 1px solid rgba(var(--warn-rgb), 0.35);
    border-radius: var(--radius);
    background: rgba(var(--warn-rgb), 0.08);
    font-size: 0.84rem;
    color: var(--text-dim);
  }
  .relocate-actions {
    display: flex;
    gap: 8px;
    margin-top: 8px;
  }
  .missing :global(svg) {
    flex-shrink: 0;
    margin-top: 2px;
    color: var(--warn);
  }
  .missing strong {
    color: var(--text);
  }
  .missing.elsewhere {
    border-color: var(--border);
    background: transparent;
  }
  .missing.elsewhere :global(svg) {
    color: var(--text-faint);
  }
  .emptied-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 10px;
  }
  .path-line {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 0 18px 50px;
  }
  .path {
    font-size: 0.8rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .path-input {
    flex: 1;
    padding: 6px 10px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.82rem;
    outline: none;
  }
  .playing {
    color: var(--success);
    font-weight: 600;
  }
</style>
