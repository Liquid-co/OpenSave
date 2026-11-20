<script context="module">
  // The tiles come in one after another the first time in a session, as the
  // library's do; coming back to the Library is not made to wait.
  let shownBefore = false;
</script>

<script>
  // The games played lately, as wide tiles: the one being played first, then
  // the rest newest first, each saying where and when (lib/libraryhome.js
  // recentlyPlayed). Another device's play is known from its save arriving.
  import { onDestroy } from 'svelte';
  import CoverImage from '../../components/CoverImage.svelte';
  import { gameCover } from '../../lib/api.js';
  import { navigate, settings } from '../../lib/stores.js';
  import { openGameMenu } from '../../lib/contextmenu.js';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { playLength } from '../../lib/format.js';
  import { t } from '../../lib/i18n.js';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import { gatedFlip, pop } from '../../lib/motion.js';

  const firstShow = !shownBefore;
  shownBefore = true;

  /** [{game, playing, at, on}] */
  export let items = [];

  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));

  // timeAgoParts reads ISO strings; the times here are milliseconds.
  const ago = (at) => {
    const p = timeAgoParts(new Date(at).toISOString(), now);
    if (p.kind === 'invalid') return p.text;
    const { kind, ...params } = p;
    return $t('timeAgo.' + kind, params);
  };

  let revealed = new Set();
  const reveal = (id) => (revealed = new Set(revealed).add(id));
  const unreveal = (id) => {
    const next = new Set(revealed);
    next.delete(id);
    revealed = next;
  };

  $: here = $settings?.deviceName ?? '';
  function line(item) {
    if (item.playing) {
      const ms = now - item.at;
      return ms >= 60_000 ? $t('home.recent.playingFor', { length: playLength(ms) }) : $t('home.recent.playingNow');
    }
    return $t('home.recent.where', { device: item.on || here, when: ago(item.at) });
  }
</script>

<section class="recent" aria-labelledby="recent-title">
  <h2 id="recent-title" class="section-title">{$t('home.recent.title')}</h2>
  <div class="row">
    {#each items as item, i (item.game.id)}
      <button
        class="tile"
        class:first={firstShow}
        style="--i: {i}"
        animate:gatedFlip
        in:pop={{ start: 0.96, delay: firstShow ? 0 : 60 }}
        class:playing={item.playing}
        on:click={() => navigate('game', { gameId: item.game.id })}
        on:contextmenu={(e) => openGameMenu(e, item.game)}
        on:mouseenter={() => reveal(item.game.id)}
        on:mouseleave={() => unreveal(item.game.id)}
        on:focus={() => reveal(item.game.id)}
        on:blur={() => unreveal(item.game.id)}
      >
        <span class="art">
          <span class="fallback">{item.game.name}</span>
          <CoverImage src={gameCover(item.game, false)} alt="" revealed={revealed.has(item.game.id)} />
        </span>
        <span class="caption">
          <span class="name">{item.game.name}</span>
          <span class="line">{#if item.playing}<Gamepad2 size={13} />{/if}{line(item, now, $t)}</span>
        </span>
      </button>
    {/each}
  </div>
</section>

<style>
  .recent {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .section-title {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .row {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 16px;
  }
  .tile {
    position: relative;
    aspect-ratio: 460 / 215;
    border: none;
    padding: 0;
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--bg-active);
    cursor: pointer;
    text-align: left;
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
    transition:
      transform 0.15s ease,
      box-shadow 0.15s ease;
  }
  .tile:hover,
  .tile:focus-visible {
    transform: translateY(-2px);
    box-shadow:
      inset 0 0 0 1px rgba(255, 255, 255, 0.16),
      var(--shadow-lift);
  }
  .tile.first {
    animation: tile-rise 0.4s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 60ms);
  }
  @keyframes tile-rise {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
  }
  /* The one being played: a green outline that breathes, slowly — it is
     live, and the only thing on the page that is. */
  .tile.playing::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: inherit;
    box-shadow: inset 0 0 0 2px var(--success);
    pointer-events: none;
    animation: breathe 2.8s ease-in-out infinite;
  }
  @keyframes breathe {
    50% {
      opacity: 0.4;
    }
  }
  .art :global(img) {
    transition: transform 0.45s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .tile:hover .art :global(img),
  .tile:focus-visible .art :global(img) {
    transform: scale(1.05);
  }
  .art {
    position: absolute;
    inset: 0;
  }
  .art :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .fallback {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    padding: 12px;
    color: var(--text-faint);
    font-weight: 700;
    text-align: center;
  }
  .caption {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    padding: 26px 12px 10px;
    background: linear-gradient(180deg, transparent, rgba(0, 0, 0, 0.8));
    display: flex;
    flex-direction: column;
    gap: 1px;
    color: #fff;
  }
  .name {
    font-weight: 700;
    font-size: var(--text-base);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    text-shadow: 0 1px 8px rgba(0, 0, 0, 0.5);
  }
  .line {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: var(--text-sm);
    color: rgba(255, 255, 255, 0.85);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .playing .line {
    color: #86efac;
  }
  /* Narrower windows show three, then two; never a half tile. */
  @media (max-width: 1180px) {
    .row {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
    .tile:nth-child(4) {
      display: none;
    }
  }
  @media (max-width: 860px) {
    .row {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .tile:nth-child(3) {
      display: none;
    }
  }
</style>
