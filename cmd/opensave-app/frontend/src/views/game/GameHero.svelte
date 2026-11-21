<script>
  // The top of a game's page: its cover and name over its own art, where the
  // save stands with your other devices, what has been played, and Play and
  // Sync now. The rest of what can be done is in the ⋯ menu.
  //
  // Underneath, anything that needs an answer first: the save emptied here,
  // its folder missing, or the same folder found on another drive.
  //
  // Motion: the cover settles in, the words and buttons arrive after it, and
  // the line under the name arrives again whenever what it says changes. Sync
  // now's arrows turn while a sync runs.
  import { createEventDispatcher, onDestroy } from 'svelte';
  import { peers, navigate, toast, syncActivity, askConfirm, conflicts, locationConflicts } from '../../lib/stores.js';
  import { emptiedWhere } from '../../lib/emptied.js';
  import { whenLabel } from '../../lib/snapshots.js';
  import { api, native, gameCover } from '../../lib/api.js';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { playLength } from '../../lib/format.js';
  import { canLaunch, cleanGameName, renameGame, MAX_GAME_NAME } from '../../lib/gameactions.js';
  import { restoreProgressText } from '../../lib/restoreprogress.js';
  import { heroStatus, playedTotal } from '../../lib/gamepage.js';
  import { collections, isFavourite, toggleFavourite } from '../../lib/collections.js';
  import { arrive } from '../../lib/motion.js';
  import { t } from '../../lib/i18n.js';
  import CoverImage from '../../components/CoverImage.svelte';
  import Dropdown from '../../components/ui/Dropdown.svelte';
  import ArrowLeft from 'lucide-svelte/icons/arrow-left';
  import Play from 'lucide-svelte/icons/play';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import MoreHorizontal from 'lucide-svelte/icons/ellipsis';
  import Pencil from 'lucide-svelte/icons/pencil';
  import Star from 'lucide-svelte/icons/star';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import Info from 'lucide-svelte/icons/info';
  import RotateCcw from 'lucide-svelte/icons/rotate-ccw';
  import CircleCheck from 'lucide-svelte/icons/circle-check';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import CircleDashed from 'lucide-svelte/icons/circle-dashed';
  import Monitor from 'lucide-svelte/icons/monitor';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import Camera from 'lucide-svelte/icons/camera';
  import SlidersHorizontal from 'lucide-svelte/icons/sliders-horizontal';
  import LoaderCircle from 'lucide-svelte/icons/loader-circle';

  export let game;
  /** The page's shared runner (see lib/runner.js). */
  export let runner;
  /** Set (to a new value each time) when "Rename…" was chosen in a menu or
   *  Ctrl+K: the name opens for editing. */
  export let renameRequest = undefined;
  /** Activity's entry for this game (lastPlayedAt, lastPlayedOn, devices). */
  export let played = null;
  const { busy, run } = runner;
  const dispatch = createEventDispatcher();

  // ── Renaming, in place ──
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
      e.stopPropagation();
      editName = false;
    }
  }
  function focusSelect(el) {
    el.focus();
    el.select();
  }

  // ── Where it stands ──
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));
  const ago = (ms) => {
    const p = timeAgoParts(new Date(ms).toISOString(), now);
    if (p.kind === 'invalid') return p.text;
    const { kind, ...params } = p;
    return $t('timeAgo.' + kind, params);
  };
  $: activity = $syncActivity[game.id];
  $: conflict = !!$conflicts[game.id] || $locationConflicts.some((c) => c.gameId === game.id);
  $: status = heroStatus({ game, peers: $peers, activity, conflict });
  $: statusText =
    status.kind === 'playing'
      ? $t('game.hero.playing')
      : status.kind === 'syncing'
        ? $t('game.hero.syncing', { n: status.percentage })
        : status.kind === 'conflict'
          ? $t('game.hero.conflict')
          : status.kind === 'synced'
            ? status.others
              ? $t('game.hero.syncedMore', { device: status.device, when: ago(status.ms), n: status.others })
              : $t('game.hero.synced', { device: status.device, when: ago(status.ms) })
            : status.kind === 'never'
              ? $t('game.hero.never')
              : $t('game.hero.alone');
  const STATUS_ICON = { playing: Gamepad2, syncing: RefreshCw, conflict: TriangleAlert, synced: CircleCheck, never: CircleDashed, alone: Monitor };

  $: total = playedTotal(played, game);
  $: lastMs = played?.lastPlayedAt || (game.lastPlayedAt ? Date.parse(game.lastPlayedAt) : 0);
  $: lastOn = played?.lastPlayedOn || $t('game.hero.thisPc');
  $: snapCount = Object.values(game.branches ?? {}).reduce((n, b) => n + (b.snapshots?.length ?? 0), 0);

  // ── Acting ──
  const syncNow = () => run($t('game.header.syncTriggered'), () => api.post(`/api/games/${game.id}/sync`));
  const launchGame = () => run($t('game.header.launching'), () => api.post(`/api/games/${game.id}/launch`));
  async function openSaveFolder() {
    const problem = await native.openFolder(game.savePath);
    if (problem) toast(problem, 'error');
  }
  let menuOpen = false;
  $: favourite = isFavourite($collections, game.id);
  $: menu = [
    { label: $t('game.hero.takeSnapshot'), icon: Camera, run: () => dispatch('snapshot') },
    { label: $t('game.header.openFolder'), icon: FolderOpen, run: openSaveFolder },
    null,
    { label: $t('common.rename'), icon: Pencil, run: startRename },
    { label: favourite ? $t('game.header.removeFavourite') : $t('game.header.addFavourite'), icon: Star, run: () => toggleFavourite($collections, game) },
    null,
    { label: $t('game.hero.settings'), icon: SlidersHorizontal, run: () => dispatch('tab', 'settings') }
  ];

  // ── Questions that come first ──
  let answering = false;
  async function answerEmptied(answer) {
    if (answer === 'delete') {
      const n = game.emptied.files;
      const ok = await askConfirm($t('game.header.deleteEverywhereMsg', { name: game.name, n }), {
        title: $t('game.header.deleteEverywhereTitle'),
        confirmText: $t('game.header.deleteEverywhereAction'),
        danger: true
      });
      if (!ok) return;
    }
    answering = true;
    try {
      const res = await api.post(`/api/games/${game.id}/emptied`, { answer });
      if (answer === 'delete') toast($t('game.header.deletedAtNextSync', { name: game.name }), 'success');
      else if (res.fetching > 0) toast($t('game.header.puttingBackDevices', { name: game.name, n: res.fetching }), 'success');
      else toast($t('game.header.filesBack', { name: game.name }), 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      answering = false;
    }
  }
  // Offered, never switched to on its own: it could be an old copy.
  let relocating = false;
  async function answerRelocation(use) {
    const found = game.relocateSuggestion;
    relocating = true;
    try {
      if (use) {
        await api.post(`/api/games/${game.id}/relocate`, { path: found });
        toast($t('game.header.relocated', { name: game.name, path: found }), 'success');
      } else {
        await api.post(`/api/games/${game.id}/relocate/dismiss`);
        toast($t('game.header.relocateDismissed', { path: found, name: game.name }), 'success');
      }
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      relocating = false;
    }
  }
</script>

<section class="hero" aria-label={game.name}>
  <button class="back" on:click={() => navigate('home')}><ArrowLeft size={16} />{$t('game.hero.library')}</button>
  <div class="row">
    <span class="cover">
      <span class="fallback">{game.name}</span>
      <CoverImage src={gameCover(game, true)} alt="" />
    </span>
    <div class="text">
      {#if editName}
        <div class="rename">
          <input
            class="name-input"
            class:invalid={!!nameProblem}
            bind:value={nameDraft}
            maxlength={MAX_GAME_NAME + 20}
            aria-label={$t('game.header.renameNameLabel')}
            aria-invalid={!!nameProblem}
            use:focusSelect
            on:keydown={nameKeys}
          />
          <button class="btn small primary" disabled={!!nameProblem || nameSaving} on:click={saveName}>{$t('game.header.renameSave')}</button>
          <button class="btn small" on:click={() => (editName = false)}>{$t('common.cancel')}</button>
        </div>
        <p class="name-hint" class:problem={!!nameProblem} role={nameProblem ? 'alert' : undefined}>{nameProblem ?? $t('game.hero.renameHint')}</p>
      {:else}
        <h1>
          <span class="name">{game.name}</span>
          <button class="icon-btn" title={$t('game.header.renameHint')} aria-label={$t('common.rename')} on:click={startRename}><Pencil size={15} /></button>
          <button
            class="icon-btn star"
            class:on={favourite}
            title={favourite ? $t('game.header.removeFavourite') : $t('game.header.addFavourite')}
            aria-label={favourite ? $t('game.header.removeFavourite') : $t('game.header.addFavourite')}
            aria-pressed={favourite}
            on:click={() => toggleFavourite($collections, game)}><Star size={17} /></button
          >
        </h1>
      {/if}
      {#key status.kind}
        <p class="status s-{status.kind}" role="status" in:arrive={{ y: 4 }}>
          {#if game.restoring}
            <span class="spin"><LoaderCircle size={15} /></span>{restoreProgressText(game.restoring, $t)}
          {:else}
            <span class="sicon"><svelte:component this={STATUS_ICON[status.kind]} size={15} /></span>{statusText}
          {/if}
        </p>
      {/key}
      <dl class="stats">
        <div><dt>{$t('game.hero.played')}</dt><dd>{total ? playLength(total) : $t('game.hero.notYet')}</dd></div>
        <div><dt>{$t('game.hero.lastPlayed')}</dt><dd>{lastMs ? $t('game.hero.lastPlayedOn', { device: lastOn, when: ago(lastMs) }) : $t('game.hero.notYet')}</dd></div>
        <div><dt>{$t('game.hero.branch')}</dt><dd><button class="link" on:click={() => dispatch('tab', 'branches')}>{game.activeBranch}</button></dd></div>
        <div><dt>{$t('game.hero.snapshots')}</dt><dd><button class="link" on:click={() => dispatch('tab', 'history')}>{snapCount}</button></dd></div>
      </dl>
    </div>
    <div class="actions">
      {#if canLaunch(game)}
        <button class="btn play" disabled={$busy} on:click={launchGame}><Play size={16} />{$t('game.hero.play')}</button>
      {/if}
      <button class="btn glass" class:turning={activity?.state === 'running'} disabled={$busy} on:click={syncNow}><span class="arrows"><RefreshCw size={15} /></span>{$t('game.header.syncNow')}</button>
      <span class="more-wrap">
        <button class="btn glass sq" aria-label={$t('game.hero.more')} aria-haspopup="menu" aria-expanded={menuOpen} on:click={() => (menuOpen = !menuOpen)}><MoreHorizontal size={18} /></button>
        <Dropdown items={menu} bind:open={menuOpen} label={$t('game.hero.more')} />
      </span>
    </div>
  </div>
</section>

{#if game.emptied && !game.savePathMissing}
  <!-- Every save file went at once here. Nothing is synced until this is
       answered: syncing it would delete them on the other devices too
       (internal/p2p/syncengine/hold.go). -->
  <div class="alert warn" role="status" in:arrive={{ y: 6 }}>
    <TriangleAlert size={18} />
    <div>
      {#if game.emptied.state === 'fetching'}
        <strong>{$t('game.header.emptiedFetchingTitle')}</strong>
        <p>{$t('game.header.emptiedFetchingBody')}</p>
      {:else}
        <strong>{$t('game.header.emptiedTitle', { where: emptiedWhere(game.emptied.locations) })}</strong>
        <p>
          {$t('game.header.emptiedBody')}
          {#if game.emptied.putBackFrom}
            {$t('game.header.putBackFromSnapshot', { when: whenLabel(game.emptied.putBackFromTime) })}
          {:else}
            {$t('game.header.putBackFromDevices')}
          {/if}
          {$t('game.header.orDeleteThere')}
        </p>
        <div class="alert-actions">
          <button class="btn small primary" disabled={answering} on:click={() => answerEmptied('restore')}><RotateCcw size={13} />{$t('game.header.putBackBtn')}</button>
          <button class="btn small" disabled={answering} on:click={() => answerEmptied('delete')}>{$t('game.header.deleteThereBtn')}</button>
        </div>
      {/if}
    </div>
  </div>
{/if}

{#if game.savePathMissing && game.saveDriveMissing}
  <!-- The drive itself isn't on this device: nothing was lost here, so this
       informs rather than warns. -->
  <div class="alert" role="status" in:arrive={{ y: 6 }}>
    <Info size={18} />
    <div>
      <strong>{$t('game.header.elsewhereTitle')}</strong>
      <p>{$t('game.header.elsewhereBody', { drive: game.savePath.slice(0, 2) })}</p>
    </div>
  </div>
{:else if game.savePathMissing}
  <!-- The folder is not there. It is not created again: an empty folder in
       its place would read as every file deleted. -->
  <div class="alert warn" role="status" in:arrive={{ y: 6 }}>
    <TriangleAlert size={18} />
    <div>
      <strong>{$t('game.header.folderMissingTitle')}</strong>
      <p>{$t('game.header.folderMissingBody')}</p>
      <div class="alert-actions"><button class="btn small" on:click={() => dispatch('tab', 'settings')}>{$t('game.hero.changeFolder')}</button></div>
    </div>
  </div>
{/if}

{#if game.savePathMissing && game.relocateSuggestion}
  <!-- Offered, not taken: the same path on another drive can be an old
       backup, and syncing from it would spread stale saves. -->
  <div class="alert" role="status" in:arrive={{ y: 6 }}>
    <Info size={18} />
    <div>
      <strong>{$t('game.header.relocateTitle', { path: game.relocateSuggestion })}</strong>
      <p>{$t('game.header.relocateBody')}</p>
      <div class="alert-actions">
        <button class="btn small primary" disabled={relocating} on:click={() => answerRelocation(true)}>{$t('game.header.relocateUse')}</button>
        <button class="btn small ghost" disabled={relocating} on:click={() => answerRelocation(false)}>{$t('game.header.relocateNotThis')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .hero {
    position: relative;
    padding: 2px 0 10px;
  }
  .back {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 30px;
    margin: -4px 0 14px -8px;
    padding: 0 10px 0 6px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-md);
    font-weight: 600;
    cursor: pointer;
    transition:
      background 0.15s,
      color 0.15s;
  }
  .back:hover {
    background: rgba(255, 255, 255, 0.08);
    color: var(--text);
  }
  .back :global(svg) {
    transition: transform 0.2s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .back:hover :global(svg) {
    transform: translateX(-3px);
  }
  .row {
    display: flex;
    align-items: flex-end;
    gap: 24px;
  }
  /* The cover settles in: from a little below and smaller, with a touch of
     overshoot. Movement only, so it is there from the first frame. */
  .cover {
    position: relative;
    width: 124px;
    aspect-ratio: 2 / 3;
    flex: none;
    border-radius: 10px;
    overflow: hidden;
    background: linear-gradient(160deg, var(--bg-active), var(--bg-raised));
    box-shadow:
      0 18px 40px rgba(0, 0, 0, 0.5),
      inset 0 0 0 1px rgba(255, 255, 255, 0.12);
    animation: cover-in 0.6s cubic-bezier(0.3, 1.3, 0.5, 1) backwards;
    transition: transform 0.3s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .cover:hover {
    transform: translateY(-3px) rotate(-1deg);
  }
  @keyframes cover-in {
    from {
      transform: translateY(18px) scale(0.92);
    }
  }
  .cover :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .fallback {
    position: absolute;
    inset: auto 0 0 0;
    padding: 10px;
    font-weight: 700;
  }
  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .text > * {
    animation: arrive 0.4s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
  }
  .text > :nth-child(2) {
    animation-delay: 60ms;
  }
  .text > :nth-child(3) {
    animation-delay: 120ms;
  }
  h1 {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    font-size: clamp(1.7rem, 3.2vw, 2.6rem);
    font-weight: 700;
    line-height: 1.08;
    letter-spacing: -0.01em;
  }
  .name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .icon-btn {
    flex: none;
    width: 30px;
    height: 30px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
    opacity: 0;
    transition:
      opacity 0.15s,
      background 0.15s,
      color 0.15s;
  }
  h1:hover .icon-btn,
  .icon-btn:focus-visible,
  .star.on {
    opacity: 1;
  }
  .icon-btn:hover {
    background: rgba(255, 255, 255, 0.08);
    color: var(--text);
  }
  .star.on {
    color: var(--warn);
  }
  .star.on :global(svg) {
    fill: currentColor;
    animation: star 0.4s cubic-bezier(0.3, 1.6, 0.5, 1);
  }
  @keyframes star {
    from {
      transform: scale(0.5) rotate(-30deg);
    }
  }
  .rename {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .name-input {
    flex: 1;
    max-width: 32em;
    font-size: var(--text-xl);
    font-weight: 600;
  }
  .name-input.invalid {
    border-color: var(--danger);
  }
  .name-hint {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .name-hint.problem {
    color: var(--danger-text);
  }
  .status {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--text-dim);
  }
  .sicon,
  .spin {
    display: grid;
  }
  .s-synced .sicon {
    color: var(--success);
  }
  .s-playing .sicon {
    color: var(--success);
  }
  .s-syncing .sicon {
    color: var(--accent-text);
    animation: turn 1.1s linear infinite;
  }
  .s-conflict {
    color: var(--warn);
  }
  .s-never .sicon,
  .s-alone .sicon {
    color: var(--text-faint);
  }
  .spin {
    color: var(--accent-text);
    animation: turn 1s linear infinite;
  }
  @keyframes turn {
    to {
      transform: rotate(360deg);
    }
  }
  .stats {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 32px;
    margin: 4px 0 0;
  }
  .stats dt {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .stats dd {
    margin: 2px 0 0;
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }
  .link:hover {
    color: var(--game-text, var(--accent-text));
  }
  .actions {
    flex: none;
    display: flex;
    gap: 8px;
    align-items: center;
    animation: arrive 0.45s cubic-bezier(0.2, 0.7, 0.2, 1) 0.16s backwards;
  }
  .actions .btn {
    height: 44px;
  }
  .play {
    padding: 0 22px;
    background: var(--game-fill, var(--accent-fill));
    border-color: var(--game-fill, var(--accent-fill));
    color: var(--game-on-fill, var(--on-accent));
    font-size: var(--text-base);
    /* The cover's colour arrives a moment after the page: the label changes
       with the fill, so the two never sit mismatched. */
    transition:
      background 0.4s,
      border-color 0.4s,
      color 0.4s,
      transform 0.18s cubic-bezier(0.2, 0.7, 0.2, 1),
      box-shadow 0.18s;
  }
  .play:hover:not(:disabled) {
    background: var(--game-fill-hover, var(--accent-fill-hover));
    border-color: var(--game-fill-hover, var(--accent-fill-hover));
    transform: translateY(-1px);
    box-shadow: 0 8px 22px rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.3);
  }
  .play :global(svg) {
    fill: currentColor;
  }
  .glass {
    background: rgba(255, 255, 255, 0.08);
    border-color: rgba(255, 255, 255, 0.14);
  }
  .glass:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.14);
  }
  :global([data-theme='light']) .glass {
    background: rgba(20, 20, 40, 0.05);
    border-color: rgba(20, 20, 40, 0.12);
  }
  .sq {
    width: 44px;
    padding: 0;
  }
  .arrows {
    display: grid;
  }
  .turning .arrows {
    animation: turn 1.1s linear infinite;
  }
  .more-wrap {
    position: relative;
  }
  .alert {
    display: flex;
    gap: 12px;
    margin-top: 12px;
    padding: 12px 14px;
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
    background: var(--glass);
    color: var(--text-dim);
    font-size: var(--text-md);
  }
  .alert > :global(svg) {
    flex: none;
    margin-top: 1px;
    color: var(--text-faint);
  }
  .alert.warn {
    border-color: rgba(var(--warn-rgb), 0.4);
    background: linear-gradient(0deg, rgba(var(--warn-rgb), 0.08), rgba(var(--warn-rgb), 0.08)), var(--glass);
  }
  .alert.warn > :global(svg) {
    color: var(--warn);
  }
  .alert strong {
    color: var(--text);
  }
  .alert p {
    margin-top: 2px;
  }
  .alert-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 10px;
  }
  /* Narrow windows and the Deck: the buttons go under the words. */
  @media (max-width: 1100px) {
    .row {
      flex-wrap: wrap;
      align-items: flex-end;
    }
    .cover {
      width: 104px;
    }
    .actions {
      width: 100%;
    }
  }
</style>
