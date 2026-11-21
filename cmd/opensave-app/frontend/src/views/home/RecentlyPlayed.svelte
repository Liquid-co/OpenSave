<script context="module">
  // The tiles come in one after another the first time in a session, as the
  // library's do; coming back to the Library is not made to wait.
  let shownBefore = false;
</script>

<script>
  // The games played lately, as wide tiles: the one being played first, then
  // the rest newest first, each saying where and when (lib/libraryhome.js
  // recentlyPlayed). Another device's play is known from its save arriving.
  //
  // When there are more than fit across, the row is a conveyor belt
  // (lib/belt.js): hold a tile and drag it sideways, and the tiles move with
  // the pointer and come round again at the other end; let go and it settles
  // on the nearest tile, a flick carrying it on. The arrow keys move it a tile
  // at a time. A press that hardly moves is still a click and opens the game.
  // The belt is driven frame by frame rather than by CSS transitions, so a
  // tile going round from one end to the other always jumps out of sight.
  import { onDestroy } from 'svelte';
  import { beltSlots, beltX, beltRest, inView, wrap } from '../../lib/belt.js';
  import { motionOn } from '../../lib/motion.js';
  import CoverImage from '../../components/CoverImage.svelte';
  import { gameCover } from '../../lib/api.js';
  import { navigate, settings, toast } from '../../lib/stores.js';
  import { openGameMenu } from '../../lib/contextmenu.js';
  import { recentHidden, hideRecent, unhideRecent } from '../../lib/recenthidden.js';
  import EyeOff from 'lucide-svelte/icons/eye-off';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { playLength } from '../../lib/format.js';
  import { t } from '../../lib/i18n.js';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import { pop } from '../../lib/motion.js';

  const firstShow = !shownBefore;
  shownBefore = true;

  /** [{game, playing, at, on}] */
  export let items = [];

  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => {
    clearInterval(tick);
    cancelAnimationFrame(frame);
  });

  // ── The belt ──
  const GAP = 16;
  let innerWidth = 1280;
  let width = 0; // the row's own width
  let row;
  $: slots = beltSlots(innerWidth);
  $: belt = items.length > slots;
  $: shown = belt ? items : items.slice(0, slots);
  $: step = width > 0 ? (width + GAP) / slots : 0;
  $: tileW = Math.max(0, step - GAP);
  $: tileH = (tileW * 215) / 460;
  $: span = shown.length * step;
  // How far the belt has moved, px; a tile's place follows from it.
  let offset = 0;
  $: if (!belt) offset = 0;
  $: place = (i) => (belt ? beltX(i, shown.length, step, offset) : i * step);

  // The newest play arriving goes back to the start, the short way round.
  let firstId = '';
  $: if ((items[0]?.game.id ?? '') !== firstId) {
    const was = firstId;
    firstId = items[0]?.game.id ?? '';
    if (was && belt && span) settle(Math.round(offset / span) * span);
  }

  let frame = 0;
  function settle(target) {
    cancelAnimationFrame(frame);
    const from = offset;
    const distance = target - from;
    const ms = $motionOn ? Math.min(420, 220 + Math.abs(distance) * 0.5) : 0;
    if (!ms || !distance) {
      offset = span ? wrap(target, span) : target;
      return;
    }
    const start = performance.now();
    const ease = (p) => 1 - Math.pow(1 - p, 3);
    const run = (t) => {
      const p = Math.min(1, (t - start) / ms);
      offset = from + distance * ease(p);
      if (p < 1) frame = requestAnimationFrame(run);
      else offset = span ? wrap(target, span) : target;
    };
    frame = requestAnimationFrame(run);
  }

  // Dragging: past a few pixels a press becomes a drag, and the click that
  // ends it does not open a game.
  let press = null; // {id, x, offset, lastX, lastT, v}
  let dragging = false;
  let swallowClick = false;
  function down(e) {
    if (!belt || e.button !== 0) return;
    cancelAnimationFrame(frame);
    press = { id: e.pointerId, x: e.clientX, offset, lastX: e.clientX, lastT: e.timeStamp, v: 0 };
  }
  function move(e) {
    if (!press || e.pointerId !== press.id) return;
    const dx = e.clientX - press.x;
    if (!dragging) {
      if (Math.abs(dx) < 6) return;
      dragging = true;
      row.setPointerCapture?.(press.id);
    }
    offset = press.offset - dx;
    const dt = e.timeStamp - press.lastT;
    if (dt > 0) press.v = 0.7 * press.v + 0.3 * ((press.lastX - e.clientX) / dt);
    press.lastX = e.clientX;
    press.lastT = e.timeStamp;
  }
  function up(e) {
    if (!press || e.pointerId !== press.id) return;
    const { v, lastT } = press;
    press = null;
    if (!dragging) return;
    dragging = false;
    // The click that ends a drag, if the browser sends one, opens nothing;
    // cleared straight after, so it can never eat the next real click.
    swallowClick = true;
    setTimeout(() => (swallowClick = false), 0);
    // Held still for a moment before letting go: no flick.
    settle(beltRest(offset, step, e.timeStamp - lastT > 80 ? 0 : v));
  }
  function clicked(e) {
    if (!swallowClick) return;
    swallowClick = false;
    e.stopPropagation();
    e.preventDefault();
  }
  function keys(e) {
    if (!belt || (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight')) return;
    e.preventDefault();
    settle(beltRest(offset, step) + (e.key === 'ArrowRight' ? step : -step));
  }

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

  // Out of the row until the game is played again, with a way back.
  function remove(item) {
    const id = item.game.id;
    const before = $recentHidden[id];
    hideRecent(id, item.at);
    toast($t('home.recent.removed', { game: item.game.name }), 'info', {
      undo: true,
      ttl: 8000,
      action: { label: $t('toast.undo'), run: () => unhideRecent(id, before) }
    });
  }
  const menu = (e, item) => openGameMenu(e, item.game, [{ label: $t('home.recent.remove'), icon: EyeOff, run: () => remove(item) }]);

  $: here = $settings?.deviceName ?? '';
  function line(item) {
    if (item.playing) {
      const ms = now - item.at;
      return ms >= 60_000 ? $t('home.recent.playingFor', { length: playLength(ms) }) : $t('home.recent.playingNow');
    }
    return $t('home.recent.where', { device: item.on || here, when: ago(item.at) });
  }
</script>

<svelte:window bind:innerWidth />

<section class="recent" aria-labelledby="recent-title">
  <h2 id="recent-title" class="section-title">{$t('home.recent.title')}</h2>
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div
    class="row"
    class:belt
    class:dragging
    bind:this={row}
    bind:clientWidth={width}
    style="height: {tileH}px"
    on:pointerdown={down}
    on:pointermove={move}
    on:pointerup={up}
    on:pointercancel={up}
    on:click|capture={clicked}
    on:keydown={keys}
    on:dragstart|preventDefault
  >
    {#each shown as item, i (item.game.id)}
      {@const x = place(i)}
      {@const seen = !belt || inView(x, tileW, width)}
      <div class="slot" style="width: {tileW}px; transform: translate3d({x}px, 0, 0)">
      <button
        class="tile"
        class:first={firstShow}
        style="--i: {i}"
        in:pop={{ start: 0.96, delay: firstShow ? 0 : 60 }}
        class:playing={item.playing}
        tabindex={seen ? 0 : -1}
        aria-hidden={seen ? undefined : 'true'}
        on:click={() => navigate('game', { gameId: item.game.id })}
        on:contextmenu={(e) => menu(e, item)}
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
      </div>
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
  /* Each tile in a slot placed by the script, so the tiles can be moved as
     one: laid out in order, or carried round on the belt. */
  .row {
    position: relative;
  }
  .slot {
    position: absolute;
    top: 0;
    left: 0;
    height: 100%;
  }
  /* Laid out in order, a new play moving the rest along glides. On the belt
     the script moves them, every frame, and nothing may glide. */
  .row:not(.belt) .slot {
    transition: transform 0.35s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .row.belt {
    cursor: grab;
    touch-action: pan-y;
    user-select: none;
    overflow: hidden;
    /* Room for a tile's hover lift and shadow, which the clip would cut. */
    padding: 6px 0;
    margin: -6px 0;
    box-sizing: content-box;
  }
  .row.belt .slot {
    top: 6px;
    height: calc(100% - 12px);
  }
  .row.dragging,
  .row.dragging .tile {
    cursor: grabbing;
  }
  .tile {
    position: relative;
    width: 100%;
    height: 100%;
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
</style>
