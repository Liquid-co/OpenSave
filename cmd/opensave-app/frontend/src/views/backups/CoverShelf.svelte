<script>
  // Every game as its cover, with a stack behind it that grows with its
  // copies in the cloud and the count on top. A game with snapshots here and
  // nothing up yet is dashed, with Upload now. Click a cover for its copies.
  //
  // Motion, all of it off with animations off (app.css):
  // - the covers arrive one after another, then the stack fans out behind
  //   each and the count settles on top, as copies piling up;
  // - the one uploading now lifts its arrow, over and over, until it is up;
  // - a game going from waiting to safe changes in place: the grey and the
  //   dashed edge fade out, the stack fans out, the count arrives;
  // - a filter moves the covers to their new places (flip).
  import { createEventDispatcher } from 'svelte';
  import ArrowUp from 'lucide-svelte/icons/arrow-up';
  import { gameCover } from '../../lib/api.js';
  import { fmtSize } from '../../lib/format.js';
  import { gameState, shelfOrder, stackEdges, newestCopy, copyTime } from '../../lib/backupsview.js';
  import { gatedFlip, gated } from '../../lib/motion.js';
  import { fade } from 'svelte/transition';
  import { t } from '../../lib/i18n.js';
  import CoverImage from '../../components/CoverImage.svelte';

  export let games = [];
  export let tracked = [];
  export let busy = false;
  /** The game whose upload is running now, if any. */
  export let uploadingId = '';
  const dispatch = createEventDispatcher();
  const leave = gated(fade);

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
  {#each shown as g, i (g.gameId)}
    {@const l = lines(g)}
    {@const edges = stackEdges(g.copies.length)}
    <div class="item" style="--i: {Math.min(i, 14)}" animate:gatedFlip={{ duration: 300 }}>
      <div class="stack st-{l.st}" class:sending={uploadingId === g.gameId}>
        {#if edges >= 2}<span class="edge e2"></span>{/if}
        {#if edges >= 1}<span class="edge e1"></span>{/if}
        <button class="cv" title={g.gameName} aria-label={$t('backups.openCopies', { game: g.gameName })} on:click={() => dispatch('open', g.gameId)}>
          <span class="name">{g.gameName}</span>
          <CoverImage src={coverOf(g)} alt="" />
        </button>
        {#if g.copies.length}
          {#key g.copies.length}<span class="count" aria-hidden="true">{g.copies.length}</span>{/key}
        {/if}
        {#if l.st === 'waiting'}
          <span class="waiting" out:leave={{ duration: 220 }}>
            <span class="up"><ArrowUp size={17} /></span>
            <button class="btn small" disabled={busy} on:click={() => dispatch('upload', g)}>{uploadingId === g.gameId ? $t('backups.sending') : $t('backups.uploadNow')}</button>
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
    animation: tile-in 0.38s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 30ms);
  }
  @keyframes tile-in {
    from {
      transform: translateY(14px) scale(0.96);
    }
  }
  .stack {
    position: relative;
    aspect-ratio: 2 / 3;
    margin: 10px 10px 0 0;
  }
  /* The copies behind the cover. They fan out after the cover lands, and
     further on hover, as if lifted to be looked through. */
  .edge {
    position: absolute;
    inset: 0;
    border-radius: calc(8px * var(--round, 1));
    background: var(--bg-active);
    border: 1px solid var(--border-strong);
    transition: transform 0.24s cubic-bezier(0.2, 0.7, 0.2, 1);
    animation: fan 0.42s cubic-bezier(0.3, 1.3, 0.5, 1) backwards;
  }
  .e1 {
    transform: translate(5px, -5px);
    animation-delay: calc(var(--i) * 30ms + 160ms);
  }
  .e2 {
    transform: translate(10px, -10px);
    opacity: 0.55;
    animation-delay: calc(var(--i) * 30ms + 220ms);
  }
  @keyframes fan {
    from {
      transform: translate(0, 0);
    }
  }
  .stack:hover .e1 {
    transform: translate(7px, -7px) rotate(1.5deg);
  }
  .stack:hover .e2 {
    transform: translate(14px, -14px) rotate(3deg);
  }
  .cv {
    position: absolute;
    inset: 0;
    padding: 0;
    border: 0;
    border-radius: calc(8px * var(--round, 1));
    overflow: hidden;
    cursor: pointer;
    background: linear-gradient(160deg, var(--bg-active), var(--bg-raised));
    box-shadow:
      inset 0 0 0 1px rgba(255, 255, 255, 0.08),
      0 8px 20px rgba(0, 0, 0, 0.35);
    /* Dashed while waiting, fading to nothing once it is up. */
    outline: 1.5px dashed transparent;
    outline-offset: 2px;
    transition:
      transform 0.22s cubic-bezier(0.2, 0.7, 0.2, 1),
      box-shadow 0.22s,
      filter 0.6s ease,
      outline-color 0.6s ease;
  }
  .stack:hover .cv {
    transform: translateY(-3px);
    box-shadow:
      inset 0 0 0 1px rgba(255, 255, 255, 0.1),
      0 14px 26px rgba(0, 0, 0, 0.42);
  }
  .cv:active {
    transform: translateY(-1px) scale(0.98);
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
    border-radius: calc(11px * var(--round, 1));
    display: grid;
    place-items: center;
    background: rgba(10, 10, 12, 0.75);
    color: #fff;
    font-size: var(--text-xs);
    font-weight: 700;
    pointer-events: none;
    animation: count-in 0.36s cubic-bezier(0.3, 1.5, 0.5, 1) backwards;
    animation-delay: calc(var(--i) * 30ms + 260ms);
  }
  @keyframes count-in {
    from {
      transform: scale(0.4);
    }
  }
  .st-waiting .cv {
    filter: grayscale(0.6) brightness(0.8);
    outline-color: rgba(var(--info-rgb), 0.6);
  }
  .waiting {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    border-radius: calc(8px * var(--round, 1));
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
  /* Going up: the arrow rises out of its circle and comes back, until done. */
  .sending .up {
    overflow: hidden;
  }
  .sending .up :global(svg) {
    animation: send 0.9s cubic-bezier(0.5, 0, 0.5, 1) infinite;
  }
  @keyframes send {
    0% {
      transform: translateY(0);
    }
    45% {
      transform: translateY(-22px);
    }
    46% {
      transform: translateY(22px);
    }
    100% {
      transform: translateY(0);
    }
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
    overflow: hidden;
  }
  .sending .behind :global(svg) {
    animation: send 0.9s cubic-bezier(0.5, 0, 0.5, 1) infinite;
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
    transition: color 0.4s;
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
