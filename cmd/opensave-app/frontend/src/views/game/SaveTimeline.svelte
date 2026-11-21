<script>
  // Where this save has been, over the last two weeks: a lane for each device
  // it was played on, with each session where it happened; a lane of its
  // snapshots, coloured as the History rows are; and a mark where a new
  // branch began. On a panel the page's art shows through.
  //
  // Every mark is placed from what was recorded (lib/gamepage.js), and
  // nothing floats: hovering shows a label in place, which takes no clicks.
  // Clicking a snapshot opens it in History; a branch mark, Branches.
  //
  // Motion: the sessions grow out from where they start, the snapshots rise
  // into place one after another left to right, and the branch line draws
  // down, as the page arrives. None of it starts invisible.
  import { createEventDispatcher, onDestroy } from 'svelte';
  import { playLanes, snapshotMarks, branchMarks, timelineWindow } from '../../lib/gamepage.js';
  import { snapshotKind } from '../../lib/snapshots.js';
  import { playLength } from '../../lib/format.js';
  import { t } from '../../lib/i18n.js';
  import GitBranch from 'lucide-svelte/icons/git-branch';

  export let game;
  /** Activity's items for this game (play sessions with their device). */
  export let items = [];
  /** This device's name. */
  export let here = '';
  /** Device name to colour slot (activityview.deviceSlots). */
  export let slots = new Map();
  const dispatch = createEventDispatcher();

  // The window moves with the clock, a little.
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 5 * 60_000);
  onDestroy(() => clearInterval(tick));
  $: win = timelineWindow(now, 14);
  $: lanes = playLanes(items, game.id, here, win);
  $: marks = snapshotMarks(game, win);
  $: branches = branchMarks(game, win);
  $: empty = !marks.length && lanes.every((l) => !l.sessions.length);

  const pct = (x) => (x * 100).toFixed(3) + '%';
  const slotOf = (device) => slots.get(device) ?? 5;
  const kindOf = (s) => {
    if (s.safety) return 'safety';
    const k = snapshotKind(s).kind;
    return k === 'manual' || k === 'session' || k === 'safety' ? k : 'auto';
  };
  const when = (ms) => new Date(ms).toLocaleString(undefined, { weekday: 'short', day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit' });
  const dayLabel = (ms, i) =>
    i === win.dayStarts.length - 1 ? $t('game.timeline.today') : new Date(ms).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });

  // The label shown while hovering: { lane, at, text }.
  let hover = null;
  const show = (lane, at, text) => (hover = { lane, at, text });
  const hide = () => (hover = null);
</script>

<section class="timeline" aria-labelledby="tl-title">
  <div class="head">
    <h2 id="tl-title">{$t('game.timeline.title')}</h2>
    <span class="cap">{$t('game.timeline.span')}</span>
    <span class="grow"></span>
    <div class="key">
      {#each lanes as l}
        <span><i class="swatch" style="background: var(--device-{slotOf(l.device)}, var(--device-other))"></i>{l.here ? $t('game.timeline.playedHere') : $t('game.timeline.playedOn', { device: l.device })}</span>
      {/each}
      <span><i class="tick-key"></i>{$t('game.timeline.snapshot')}</span>
      {#if branches.length}<span class="branch-key"><GitBranch size={13} />{$t('game.timeline.newBranch')}</span>{/if}
    </div>
  </div>

  <div class="grid" role="group" aria-label={$t('game.timeline.aria')}>
    <!-- First, so it lies under the lanes: the day lines and branch lines,
         across every lane. -->
    <div class="lines" aria-hidden="true" style="grid-row: 1 / span {lanes.length + 1}">
      {#each win.dayStarts as ms, i}{#if i}<span class="dayline" style="left: {pct(i / win.dayStarts.length)}"></span>{/if}{/each}
      {#each branches as b}<span class="branchline" style="left: {pct(b.at)}"></span>{/each}
    </div>
    {#each lanes as l, li (l.device)}
      <span class="label">{l.here ? $t('game.timeline.thisPc') : l.device}</span>
      <div class="track lane" style="--lane: {li}">
        {#each l.sessions as s, si}
          <span
            class="session"
            role="img"
            aria-label={$t('game.timeline.sessionTip', { device: l.here ? $t('game.timeline.thisPc') : l.device, when: when(s.startMs), length: playLength(s.endMs - s.startMs) })}
            style="left: {pct(s.at)}; width: max(4px, {pct(s.width)}); background: var(--device-{slotOf(l.device)}, var(--device-other)); --d: {si}"
            on:mouseenter={() => show(li, s.at + s.width / 2, $t('game.timeline.sessionTip', { device: l.here ? $t('game.timeline.thisPc') : l.device, when: when(s.startMs), length: playLength(s.endMs - s.startMs) }))}
            on:mouseleave={hide}
          ></span>
        {/each}
        {#if hover?.lane === li}
          <span class="tip" style="--at: {pct(hover.at)}">{hover.text}</span>
        {/if}
      </div>
    {/each}

    <span class="label">{$t('game.timeline.snapshots')}</span>
    <div class="track ticks">
      {#each marks as m, mi (m.snap.id)}
        <button
          class="tick k-{kindOf(m.snap)}"
          style="left: {pct(m.at)}; --d: {Math.min(mi, 30)}"
          aria-label={$t('game.timeline.snapshotTip', { title: snapshotKind(m.snap).title, when: when(m.ms) })}
          on:mouseenter={() => show('snaps', m.at, $t('game.timeline.snapshotTip', { title: snapshotKind(m.snap).title, when: when(m.ms) }))}
          on:mouseleave={hide}
          on:focus={() => show('snaps', m.at, $t('game.timeline.snapshotTip', { title: snapshotKind(m.snap).title, when: when(m.ms) }))}
          on:blur={hide}
          on:click={() => dispatch('snapshot', m.snap.id)}
        ></button>
      {/each}
      {#each branches as b}
        <button class="branch" style="left: {pct(b.at)}" aria-label={$t('game.timeline.branchTip', { name: b.name })} on:click={() => dispatch('branches')} on:mouseenter={() => show('snaps', b.at, $t('game.timeline.branchTip', { name: b.name }))} on:mouseleave={hide}>
          <GitBranch size={12} />
        </button>
      {/each}
      {#if hover?.lane === 'snaps'}
        <span class="tip" style="--at: {pct(hover.at)}">{hover.text}</span>
      {/if}
    </div>

    <span></span>
    <div class="track days" aria-hidden="true">
      {#each win.dayStarts as ms, i}
        {#if i % 2 === 1 || i === win.dayStarts.length - 1}
          <span class="day" class:today={i === win.dayStarts.length - 1} style="left: {pct((i + 0.5) / win.dayStarts.length)}">{dayLabel(ms, i)}</span>
        {/if}
      {/each}
    </div>

  </div>
  {#if empty}<p class="empty">{$t('game.timeline.empty')}</p>{/if}
</section>

<style>
  .timeline {
    position: relative;
    padding: 12px 18px 10px;
    border-radius: var(--radius-lg);
    background: var(--glass);
    border: 1px solid rgba(255, 255, 255, 0.07);
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  :global([data-theme='light']) .timeline {
    border-color: var(--border);
  }
  .head {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }
  h2 {
    font-size: var(--text-base);
    font-weight: 600;
  }
  .cap {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .grow {
    flex: 1;
  }
  .key {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 14px;
  }
  .key span {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: var(--text-sm);
    color: var(--text-dim);
  }
  .swatch {
    width: 10px;
    height: 10px;
    border-radius: calc(2px * var(--round, 1));
  }
  .tick-key {
    width: 2px;
    height: 12px;
    border-radius: calc(1px * var(--round, 1));
    background: var(--text-dim);
  }
  .branch-key {
    color: var(--warn) !important;
  }
  .grid {
    position: relative;
    display: grid;
    grid-template-columns: 104px 1fr;
    row-gap: 8px;
    align-items: center;
  }
  .label {
    font-size: var(--text-md);
    color: var(--text-dim);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    padding-right: 8px;
  }
  .track {
    position: relative;
    grid-column: 2;
    height: 18px;
  }
  .lane {
    border-radius: calc(4px * var(--round, 1));
    background: rgba(255, 255, 255, 0.05);
  }
  :global([data-theme='light']) .lane {
    background: rgba(20, 20, 40, 0.05);
  }
  /* A session grows out from where it began. */
  .session {
    position: absolute;
    top: 3px;
    bottom: 3px;
    border-radius: calc(3px * var(--round, 1));
    transform-origin: left;
    animation: grow 0.5s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--lane) * 90ms + var(--d) * 30ms + 120ms);
    transition: filter 0.15s;
  }
  .session:hover {
    filter: brightness(1.25);
  }
  @keyframes grow {
    from {
      transform: scaleX(0.15);
    }
  }
  .ticks {
    border-bottom: 1px solid var(--border-strong);
  }
  /* A snapshot: a short line in its kind's colour, rising into place. */
  .tick {
    position: absolute;
    bottom: 0;
    width: 3px;
    height: 14px;
    margin-left: -1px;
    padding: 0;
    border: 0;
    border-radius: calc(2px * var(--round, 1)) calc(2px * var(--round, 1)) 0 0;
    background: var(--text-faint);
    cursor: pointer;
    transform-origin: bottom;
    animation: rise-tick 0.4s cubic-bezier(0.3, 1.4, 0.5, 1) backwards;
    animation-delay: calc(var(--d) * 18ms + 240ms);
    transition:
      height 0.15s,
      filter 0.15s;
  }
  .tick:hover,
  .tick:focus-visible {
    height: 18px;
    filter: brightness(1.3);
    outline: none;
  }
  @keyframes rise-tick {
    from {
      transform: scaleY(0.25);
    }
  }
  .k-manual {
    background: var(--accent);
  }
  .k-session {
    background: var(--success);
  }
  .k-safety {
    background: var(--warn);
  }
  .branch {
    position: absolute;
    top: -8px;
    width: 20px;
    height: 20px;
    margin-left: -10px;
    display: grid;
    place-items: center;
    padding: 0;
    border-radius: 50%;
    border: 1px solid rgba(var(--warn-rgb), 0.6);
    background: var(--bg-raised);
    color: var(--warn);
    cursor: pointer;
    z-index: 1;
  }
  .lines {
    position: absolute;
    grid-column: 2;
    inset: 0;
    pointer-events: none;
  }
  .dayline {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 1px;
    background: rgba(255, 255, 255, 0.05);
  }
  :global([data-theme='light']) .dayline {
    background: rgba(20, 20, 40, 0.06);
  }
  .branchline {
    position: absolute;
    top: -4px;
    bottom: 0;
    border-left: 1px dashed var(--warn);
    transform-origin: top;
    animation: draw 0.6s cubic-bezier(0.2, 0.7, 0.2, 1) 0.3s backwards;
  }
  @keyframes draw {
    from {
      transform: scaleY(0.2);
    }
  }
  .days {
    height: 14px;
  }
  .day {
    position: absolute;
    top: 0;
    transform: translateX(-50%);
    font-size: var(--text-xs);
    color: var(--text-faint);
    white-space: nowrap;
  }
  .day.today {
    color: var(--game-text, var(--accent-text));
    font-weight: 700;
  }
  /* The label while hovering: in place above the mark, never in the way. */
  .tip {
    position: absolute;
    bottom: calc(100% + 6px);
    left: clamp(0px, calc(var(--at) - 120px), calc(100% - 240px));
    width: max-content;
    max-width: 240px;
    padding: 5px 9px;
    border-radius: calc(7px * var(--round, 1));
    background: var(--bg-active);
    border: 1px solid var(--border-strong);
    box-shadow: var(--shadow);
    font-size: var(--text-sm);
    color: var(--text);
    pointer-events: none;
    z-index: 3;
    white-space: normal;
  }
  .empty {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
</style>
