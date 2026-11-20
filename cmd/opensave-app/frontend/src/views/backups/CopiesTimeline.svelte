<script>
  // Every copy in the cloud as a dot on the last two weeks, one lane per game,
  // coloured by why it was taken (the snapshot rows' colours). The newest
  // copy of each game is ringed. Pick a dot to put that copy back.
  //
  // Cloud copies do not record which device made them, so the dots say kind,
  // not device; a copy this PC does not hold is drawn hollow, kind unknown.
  import { createEventDispatcher, onDestroy } from 'svelte';
  import { gameCover } from '../../lib/api.js';
  import { fmtSize } from '../../lib/format.js';
  import { snapshotKind } from '../../lib/snapshots.js';
  import { laneDots, placeIn, gameState, shelfOrder, newestCopy, copyTime } from '../../lib/backupsview.js';
  import { pop } from '../../lib/motion.js';
  import { t } from '../../lib/i18n.js';
  import CoverImage from '../../components/CoverImage.svelte';

  export let games = [];
  export let tracked = [];
  /** lib/backupsview.js timelineWindow(). */
  export let win;
  const dispatch = createEventDispatcher();

  $: lanes = shelfOrder(games).map((g) => ({ g, st: gameState(g), dots: laneDots(g, win) }));
  const coverOf = (g) => gameCover(tracked.find((x) => x.id === g.gameId) ?? { name: g.gameName, appId: g.appId }, true);

  const dayFmt = (ms, first) =>
    first ? new Date(ms).toLocaleDateString(undefined, { day: 'numeric', month: 'short' }) : String(new Date(ms).getDate());
  const at = (ms) => (placeIn(win, ms) * 100).toFixed(3) + '%';

  function laneSub(l) {
    if (l.st === 'waiting') return $t('backups.state.waiting');
    if (l.st === 'empty') return $t('backups.state.empty');
    const n = l.g.copies.length;
    if (!l.dots.length && n) {
      const newest = new Date(copyTime(newestCopy(l.g)));
      return $t('backups.lane.older', { n, date: newest.toLocaleDateString(undefined, { day: 'numeric', month: 'short' }) });
    }
    return $t('backups.lane.copies', { n });
  }

  // ── The dot picked ──
  let picked = null; // { laneIndex, dot }
  function pick(i, d) {
    picked = picked?.dot === d ? null : { laneIndex: i, dot: d };
  }
  function about(d) {
    const when = new Date(d.ms).toLocaleString(undefined, { day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit' });
    const size = fmtSize(d.copy.sizeBytes);
    if (d.kind === 'unknown') return { title: $t('backups.tip.unknownTitle'), line: $t('backups.tip.unknownLine', { when, size }) };
    const k = snapshotKind({ comment: d.copy.comment ?? '', isSystemAuto: !!d.copy.isSystemAuto });
    return { title: k.title, line: $t('backups.tip.line', { kind: $t('backups.kind.' + d.kind), when, size }) };
  }
  function onKey(e) {
    if (e.key === 'Escape') picked = null;
  }
  function onDocClick(e) {
    if (picked && !e.target.closest?.('.dot, .tip')) picked = null;
  }
  document.addEventListener('click', onDocClick, true);
  onDestroy(() => document.removeEventListener('click', onDocClick, true));
</script>

<svelte:window on:keydown={onKey} />

<div class="card" role="group" aria-label={$t('backups.timelineLabel')}>
  <div class="head">
    <span class="span-label">{$t('backups.lastTwoWeeks')}</span>
    <div class="track">
      {#each win.dayStarts as ms, i}
        <span class="day" class:today={i === win.dayStarts.length - 1} style="left: {at(ms + 12 * 3_600_000)}">
          {i === win.dayStarts.length - 1 ? $t('backups.today') : dayFmt(ms, i === 0)}
        </span>
      {/each}
    </div>
  </div>
  <div class="lines" aria-hidden="true">
    {#each win.dayStarts as ms}<span class="tick" style="left: {at(ms)}"></span>{/each}
    <span class="now" style="left: {at(win.now)}"></span>
  </div>

  {#each lanes as l, i (l.g.gameId)}
    <div class="lane">
      <button class="who" on:click={() => dispatch('open', l.g.gameId)} title={$t('backups.openCopies', { game: l.g.gameName })}>
        <span class="mini"><CoverImage src={coverOf(l.g)} alt="" /></span>
        <span class="who-text"><b>{l.g.gameName}</b><span class="lane-sub st-{l.st}">{laneSub(l)}</span></span>
      </button>
      <div class="track">
        {#each l.dots as d (d.copy.name)}
          <button
            class="dot k-{d.kind}"
            class:latest={d.latest}
            class:picked={picked?.dot === d}
            style="left: {(d.at * 100).toFixed(3)}%"
            aria-label={`${l.g.gameName}, ${new Date(d.ms).toLocaleString()}`}
            on:click={() => pick(i, d)}
          ></button>
        {/each}
        {#if l.st === 'waiting'}
          <span class="dot waiting" style="left: {at(win.now)}" title={$t('backups.state.waiting')}></span>
        {/if}
        {#if picked?.laneIndex === i}
          {@const info = about(picked.dot)}
          <div
            class="tip"
            class:above={i > 2 && i >= lanes.length - 3}
            role="dialog"
            aria-label={info.title}
            style="--at: {(picked.dot.at * 100).toFixed(3)}%"
            in:pop={{ start: 0.96 }}
          >
            <div class="tip-title"><i class="kdot k-{picked.dot.kind}"></i><b>{info.title}</b></div>
            <p>{info.line}</p>
            <div class="tip-actions">
              <button class="btn small primary" disabled={!l.g.tracked} title={l.g.tracked ? '' : $t('backups.trackFirst')} on:click={() => dispatch('putBack', { game: l.g, copy: picked.dot.copy })}>{$t('backups.putBackThis')}</button>
              <button class="btn small" on:click={() => dispatch('open', l.g.gameId)}>{$t('backups.details')}</button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  {/each}
</div>

<style>
  .card {
    --name-w: 196px;
    --pad-r: 18px;
    position: relative;
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
    background: var(--bg-raised);
  }
  .head,
  .lane {
    display: grid;
    grid-template-columns: var(--name-w) 1fr;
    padding-right: var(--pad-r);
  }
  .head {
    height: 32px;
    align-items: center;
  }
  .span-label {
    padding-left: 14px;
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-faint);
  }
  .track {
    position: relative;
    height: 100%;
  }
  .day {
    position: absolute;
    top: 50%;
    transform: translate(-50%, -50%);
    font-size: var(--text-sm);
    color: var(--text-faint);
    white-space: nowrap;
  }
  .day.today {
    color: var(--accent-text);
    font-weight: 700;
  }
  /* The day lines and now, across every lane. */
  .lines {
    position: absolute;
    top: 32px;
    bottom: 0;
    left: var(--name-w);
    right: var(--pad-r);
    pointer-events: none;
  }
  .tick {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 1px;
    background: var(--border);
  }
  .now {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 2px;
    margin-left: -1px;
    background: var(--accent-text);
    opacity: 0.7;
  }
  .lane {
    height: 44px;
    border-top: 1px solid var(--border);
  }
  .who {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
    padding: 0 10px 0 14px;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .who:hover b {
    text-decoration: underline;
  }
  .mini {
    position: relative;
    width: 22px;
    height: 30px;
    flex: none;
    border-radius: 3px;
    overflow: hidden;
    background: var(--bg-active);
  }
  .mini :global(img) {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .who-text {
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .who-text b {
    font-size: var(--text-md);
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .lane-sub {
    font-size: var(--text-xs);
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .lane-sub.st-waiting {
    color: var(--info);
  }
  .dot {
    position: absolute;
    top: 50%;
    width: 10px;
    height: 10px;
    padding: 0;
    border: 0;
    border-radius: 50%;
    transform: translate(-50%, -50%);
    cursor: pointer;
    transition: transform 0.15s;
  }
  .dot:hover,
  .dot:focus-visible {
    transform: translate(-50%, -50%) scale(1.35);
    outline: none;
  }
  .dot.latest {
    width: 13px;
    height: 13px;
    border: 2px solid var(--text);
    box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.12);
  }
  .dot.picked {
    width: 15px;
    height: 15px;
    border: 2px solid var(--text);
    box-shadow: 0 0 0 5px rgba(var(--accent-rgb), 0.35);
  }
  .k-session {
    background: var(--success);
  }
  .k-manual {
    background: var(--accent);
  }
  .k-safety {
    background: var(--warn);
  }
  .k-auto {
    background: var(--text-faint);
  }
  .k-unknown {
    background: transparent;
    border: 1.5px solid var(--text-faint);
  }
  .dot.waiting {
    width: 13px;
    height: 13px;
    border: 2px dashed var(--info);
    background: transparent;
    cursor: default;
  }
  .tip {
    position: absolute;
    z-index: 5;
    top: calc(100% + 4px);
    left: clamp(0px, calc(var(--at) - 120px), calc(100% - 240px));
    width: 240px;
    padding: 12px;
    border-radius: 12px;
    background: var(--bg-active);
    border: 1px solid var(--border-strong);
    box-shadow: var(--shadow);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .tip.above {
    top: auto;
    bottom: calc(100% + 4px);
  }
  .tip-title {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .tip-title b {
    font-size: var(--text-md);
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .kdot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex: none;
  }
  .tip p {
    font-size: var(--text-sm);
    color: var(--text-dim);
  }
  .tip-actions {
    display: flex;
    gap: 6px;
  }
  .tip-actions .primary {
    flex: 1;
  }
</style>
