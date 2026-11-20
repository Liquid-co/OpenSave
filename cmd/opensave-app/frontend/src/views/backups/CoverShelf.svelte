<script>
  // Every game as its cover, with a stack behind it that grows with its
  // copies in the cloud and the count on top. A game with snapshots here and
  // nothing up yet is dashed, with Upload now. Click a cover for its copies.
  import { createEventDispatcher } from 'svelte';
  import ArrowUp from 'lucide-svelte/icons/arrow-up';
  import { gameCover } from '../../lib/api.js';
  import { fmtSize } from '../../lib/format.js';
  import { gameState, shelfOrder, stackEdges, newestCopy, copyTime } from '../../lib/backupsview.js';
  import { gatedFlip, rise } from '../../lib/motion.js';
  import { t } from '../../lib/i18n.js';
  import CoverImage from '../../components/CoverImage.svelte';

  export let games = [];
  export let tracked = [];
  export let busy = false;
  const dispatch = createEventDispatcher();

  $: shown = shelfOrder(games);
  const coverOf = (g) => gameCover(tracked.find((x) => x.id === g.gameId) ?? { name: g.gameName, appId: g.appId }, true);

  /** "Today, 3:35 PM", "Yesterday", "Monday", "16 Nov". */
  function when(ms) {
    const d = new Date(ms);
    const now = new Date();
    const days = Math.round((new Date(now).setHours(0, 0, 0, 0) - new Date(ms).setHours(0, 0, 0, 0)) / 86_400_000);
    if (days === 0) return $t('backups.todayAt', { time: d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }) });
    if (days === 1) return $t('backups.yesterday');
    if (days > 1 && days < 7) return d.toLocaleDateString(undefined, { weekday: 'long' });
    return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', ...(d.getFullYear() === now.getFullYear() ? {} : { year: 'numeric' }) });
  }

  function lines(g) {
    const st = gameState(g);
    const newest = newestCopy(g);
    const copies = $t('backups.copies', { n: g.copies.length, size: fmtSize(g.totalSize) });
    if (st === 'waiting') return { st, top: $t('backups.state.waiting'), sub: $t('backups.localSnaps', { n: g.localSnapshots }) };
    if (st === 'empty') return { st, top: $t('backups.state.empty'), sub: '' };
    if (st === 'behind') return { st, top: when(copyTime(newest)), sub: $t('backups.state.behind') };
    if (st === 'cloudOnly') return { st, top: when(copyTime(newest)), sub: $t('backups.state.cloudOnly') };
    return { st, top: when(copyTime(newest)), sub: copies };
  }
</script>

<div class="shelf">
  {#each shown as g (g.gameId)}
    {@const l = lines(g)}
    <div class="item" animate:gatedFlip={{ duration: 260 }} in:rise={{ y: 6 }}>
      <div class="stack st-{l.st}">
        {#if stackEdges(g.copies.length) >= 2}<span class="edge e2"></span>{/if}
        {#if stackEdges(g.copies.length) >= 1}<span class="edge e1"></span>{/if}
        <button class="cv" title={g.gameName} aria-label={$t('backups.openCopies', { game: g.gameName })} on:click={() => dispatch('open', g.gameId)}>
          <span class="name">{g.gameName}</span>
          <CoverImage src={coverOf(g)} alt="" />
        </button>
        {#if g.copies.length}<span class="count" aria-hidden="true">{g.copies.length}</span>{/if}
        {#if l.st === 'waiting'}
          <span class="waiting">
            <span class="up"><ArrowUp size={17} /></span>
            <button class="btn small" disabled={busy} on:click={() => dispatch('upload', g)}>{$t('backups.uploadNow')}</button>
          </span>
        {:else if l.st === 'behind'}
          <span class="behind" title={$t('backups.state.behind')}><ArrowUp size={13} /></span>
        {/if}
      </div>
      <div class="meta">
        <div class="top st-{l.st}">{l.top}</div>
        {#if l.sub}<div class="sub">{l.sub}</div>{/if}
      </div>
    </div>
  {/each}
</div>
<p class="hint">{$t('backups.shelfHint')}</p>

<style>
  .shelf {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(108px, 1fr));
    gap: 8px 16px;
  }
  .item {
    display: flex;
    flex-direction: column;
    gap: 7px;
    min-width: 0;
  }
  .stack {
    position: relative;
    aspect-ratio: 2 / 3;
    margin: 10px 10px 0 0;
  }
  .edge {
    position: absolute;
    inset: 0;
    border-radius: 8px;
    background: var(--bg-active);
    border: 1px solid var(--border-strong);
    transition: transform 0.2s;
  }
  .e1 {
    transform: translate(5px, -5px);
  }
  .e2 {
    transform: translate(10px, -10px);
    opacity: 0.55;
  }
  .stack:hover .e1 {
    transform: translate(7px, -7px);
  }
  .stack:hover .e2 {
    transform: translate(14px, -14px);
  }
  .cv {
    position: absolute;
    inset: 0;
    padding: 0;
    border: 0;
    border-radius: 8px;
    overflow: hidden;
    cursor: pointer;
    background: linear-gradient(160deg, var(--bg-active), var(--bg-raised));
    box-shadow:
      inset 0 0 0 1px rgba(255, 255, 255, 0.08),
      0 8px 20px rgba(0, 0, 0, 0.35);
    transition: transform 0.2s;
  }
  .stack:hover .cv {
    transform: translateY(-2px);
  }
  .cv:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 3px;
  }
  .cv :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  /* Before the art, which covers it: it shows only where there is none. */
  .name {
    position: absolute;
    inset: auto 0 0 0;
    padding: 9px;
    color: var(--text);
    font-size: var(--text-md);
    font-weight: 700;
    text-align: left;
    line-height: 1.2;
  }
  .count {
    position: absolute;
    top: 7px;
    right: 7px;
    min-width: 24px;
    height: 22px;
    padding: 0 7px;
    border-radius: 11px;
    display: grid;
    place-items: center;
    background: rgba(10, 10, 12, 0.75);
    color: #fff;
    font-size: var(--text-xs);
    font-weight: 700;
    pointer-events: none;
  }
  .st-waiting .cv {
    filter: grayscale(0.6) brightness(0.8);
    outline: 1.5px dashed rgba(var(--info-rgb), 0.6);
    outline-offset: 2px;
  }
  .waiting {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    border-radius: 8px;
    background: rgba(10, 10, 12, 0.5);
    pointer-events: none;
  }
  .waiting .btn {
    pointer-events: auto;
  }
  .up {
    width: 34px;
    height: 34px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: rgba(var(--info-rgb), 0.18);
    color: var(--info);
  }
  .behind {
    position: absolute;
    top: 7px;
    left: 7px;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: rgba(10, 10, 12, 0.75);
    color: var(--info);
    pointer-events: none;
  }
  .meta {
    min-width: 0;
  }
  .top {
    font-size: var(--text-sm);
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .top.st-waiting {
    color: var(--info);
  }
  .top.st-empty,
  .top.st-cloudOnly {
    color: var(--text-dim);
  }
  .sub {
    font-size: var(--text-sm);
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .hint {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
</style>
