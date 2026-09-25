<script>
  // Snapshots taken on each of the last 14 days, across every game: whether
  // your saves are being kept, at a glance, and which days you played. One
  // series, so the title names it and there is no legend; the busiest day is
  // labelled, the rest are on hover or focus, and every value is in the table
  // a screen reader is given.
  import { onDestroy } from 'svelte';
  import { games } from '../../lib/stores.js';
  import { snapshotsPerDay } from '../../lib/overview.js';

  const DAYS = 14;
  let now = Date.now();
  // Once a minute is plenty to notice the day turning over.
  const tick = setInterval(() => (now = Date.now()), 60_000);
  onDestroy(() => clearInterval(tick));

  $: days = snapshotsPerDay($games, { days: DAYS, now: new Date(now) });
  $: total = days.reduce((n, d) => n + d.count, 0);
  $: played = new Set(days.flatMap((d) => d.games.map((g) => g.name))).size;
  $: max = Math.max(1, ...days.map((d) => d.count));
  // The busiest day — the latest of them, on a tie — is the one labelled.
  $: peak = days.reduce((best, d, i) => (d.count > 0 && d.count >= (days[best]?.count ?? 0) ? i : best), -1);

  let active = -1;
  const longDay = (d) => d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' });
  const shortDay = (d) => d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
  const snaps = (n) => `${n} ${n === 1 ? 'snapshot' : 'snapshots'}`;
  // The tooltip sits beside the day it describes, on the side with room, so
  // it covers neither that day's bar nor anything outside the chart.
  $: tipLeft = active >= DAYS / 2;
  $: tipX = ((tipLeft ? active : active + 1) / DAYS) * 100;
</script>

<section class="panel">
  <header>
    <h3>Snapshots, last 14 days</h3>
    <span class="total">
      {#if total}<strong>{total}</strong> across {played} {played === 1 ? 'game' : 'games'}{:else}None yet{/if}
    </span>
  </header>

  <div class="plot" role="group" aria-label="Snapshots per day, last 14 days">
    {#each days as d, i}
      <button
        class="band"
        class:active={active === i}
        aria-label="{longDay(d.date)}: {snaps(d.count)}"
        on:mouseenter={() => (active = i)}
        on:mouseleave={() => (active = -1)}
        on:focus={() => (active = i)}
        on:blur={() => (active = -1)}
      >
        {#if i === peak}<span class="peak">{d.count}</span>{/if}
        <span class="bar" style="height: {d.count ? Math.max(6, (d.count / max) * 100) : 0}%"></span>
      </button>
    {/each}

    {#if active >= 0}
      <div class="tip" class:to-left={tipLeft} style="left: {tipX}%" aria-hidden="true">
        <strong>{snaps(days[active].count)}</strong>
        <span class="tip-day">{longDay(days[active].date)}</span>
        {#each days[active].games.slice(0, 3) as g}
          <span class="tip-game"><span class="name">{g.name}</span><span>{g.count}</span></span>
        {/each}
        {#if days[active].games.length > 3}
          <span class="tip-more">and {days[active].games.length - 3} more</span>
        {/if}
      </div>
    {/if}
  </div>
  <div class="axis" aria-hidden="true">
    <span>{shortDay(days[0].date)}</span>
    <span>Today</span>
  </div>

  <table class="sr-only">
    <caption>Snapshots per day, last 14 days</caption>
    <thead><tr><th>Day</th><th>Snapshots</th></tr></thead>
    <tbody>
      {#each days as d}
        <tr><td>{longDay(d.date)}</td><td>{d.count}</td></tr>
      {/each}
    </tbody>
  </table>
</section>

<style>
  .panel {
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 14px 16px 10px;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 10px;
  }
  h3 {
    font-size: 0.88rem;
    font-weight: 600;
  }
  .total {
    font-size: 0.78rem;
    color: var(--text-faint);
    white-space: nowrap;
  }
  .total strong {
    color: var(--text);
    font-weight: 600;
  }

  /* The bars stand on a hairline baseline; the space above the tallest is
     where the busiest day's number sits. */
  .plot {
    position: relative;
    flex: 1;
    min-height: 96px;
    display: grid;
    grid-template-columns: repeat(14, 1fr);
    align-items: stretch;
    padding-top: 16px;
    border-bottom: 1px solid var(--border-strong);
  }
  .band {
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
    align-items: center;
    gap: 3px;
    min-width: 0;
    padding: 0;
    border: none;
    background: none;
    cursor: default;
    border-radius: 4px 4px 0 0;
  }
  .band:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
  }
  /* Grows up from the baseline when the panel first appears; still with
     animations off (app.css). */
  .bar {
    width: min(14px, 62%);
    border-radius: 4px 4px 0 0;
    /* The accent at full strength: every accent clears 3:1 against the panel
       in both themes, and a paler bar did not. */
    background: var(--accent);
    transform-origin: bottom;
    animation: grow 0.5s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    transition:
      background 0.12s,
      height 0.4s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .band.active .bar {
    background: var(--accent-hover);
  }
  .peak {
    font-size: 0.7rem;
    color: var(--text-dim);
    font-variant-numeric: tabular-nums;
    line-height: 1;
  }
  @keyframes grow {
    from {
      transform: scaleY(0);
    }
  }

  .axis {
    display: flex;
    justify-content: space-between;
    padding-top: 5px;
    font-size: 0.7rem;
    color: var(--text-faint);
  }

  .tip {
    position: absolute;
    top: 4px;
    transform: translateX(6px);
    z-index: 5;
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 150px;
    padding: 8px 10px;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    box-shadow: var(--shadow);
    font-size: 0.76rem;
    color: var(--text-dim);
    pointer-events: none;
  }
  .tip.to-left {
    transform: translateX(calc(-100% - 6px));
  }
  .tip strong {
    color: var(--text);
    font-size: 0.84rem;
  }
  .tip-day {
    color: var(--text-faint);
    margin-bottom: 3px;
  }
  .tip-game {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }
  .tip-game .name {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .tip-more {
    color: var(--text-faint);
  }
</style>
