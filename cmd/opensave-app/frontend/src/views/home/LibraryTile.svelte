<script>
  // One game in the library, as a wide banner or as tall box art.
  //
  // Tall art is not there for every game: Steam has a banner for everything
  // but a library capsule only for some, and a custom cover is whatever shape
  // someone found. Art whose shape is far from the tile's is shown whole over
  // a blurred copy of itself rather than cropped to a sliver of its middle.
  import { createEventDispatcher } from 'svelte';
  import Check from 'lucide-svelte/icons/check';
  import CoverImage from '../../components/CoverImage.svelte';
  import { coverURL, gameCover, isDaemonURL } from '../../lib/api.js';
  import { COVER_STYLES } from '../../lib/libraryview.js';
  import { collections, isFavourite } from '../../lib/collections.js';
  import Star from 'lucide-svelte/icons/star';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import { openGameMenu } from '../../lib/contextmenu.js';
  import { playLength } from '../../lib/format.js';

  export let game;
  export let status;
  /** 'wide' or 'tall'. */
  export let cover = 'wide';
  export let selecting = false;
  export let selected = false;
  /** Its place in the first showing of the library, to come in after the
   *  ones before it; -1 to simply be there. */
  export let enter = -1;

  const dispatch = createEventDispatcher();

  // A custom cover wins in either style; otherwise Steam's art in the shape
  // asked for, which for tall art may still come back as the banner.
  $: custom = game.coverUrl && !game.coverUrl.includes('steamstatic.com') ? game.coverUrl : '';
  $: src = custom || (cover === 'tall' ? coverURL(game.appId, true, game.name) : gameCover(game));

  const frameAspect = { wide: 460 / 215, tall: 600 / 900 };
  let letterbox = false;
  let backdrop = '';
  $: if (src || cover) {
    letterbox = false;
    backdrop = '';
  }
  function measure(e) {
    const img = e.currentTarget;
    if (!img?.naturalWidth || !img?.naturalHeight) return;
    const ratio = img.naturalWidth / img.naturalHeight / frameAspect[cover];
    letterbox = ratio > 1.33 || ratio < 0.75;
    backdrop = letterbox ? img.src : '';
  }

  // Explicit covers are blurred until the pointer is on the tile.
  let revealed = false;

  $: snapshots = Object.values(game.branches ?? {}).reduce((n, b) => n + (b.snapshots?.length ?? 0), 0);

  // Held down, a tile starts selecting (the library's `longpress`), the way a
  // phone's photo grid does — the Select button is still there, but a long
  // press is where a hand goes first. It is a long press only while the
  // pointer stays put: a drag or a scroll is not one. The click that ends it
  // is not also an open.
  const LONG_PRESS_MS = 450;
  const SLOP = 8;
  let pressTimer = null;
  let pressAt = null;
  let pressing = false;
  let longPressed = false;
  let longPressedAt = 0;
  function pointerDown(e) {
    if (e.button !== 0) return;
    pressAt = { x: e.clientX, y: e.clientY };
    pressing = true;
    clearTimeout(pressTimer);
    pressTimer = setTimeout(() => {
      pressTimer = null;
      pressing = false;
      longPressed = true;
      longPressedAt = Date.now();
      globalThis.navigator?.vibrate?.(12);
      dispatch('longpress');
    }, LONG_PRESS_MS);
  }
  function pointerMove(e) {
    if (pressTimer && pressAt && Math.hypot(e.clientX - pressAt.x, e.clientY - pressAt.y) > SLOP) endPress();
  }
  function endPress() {
    clearTimeout(pressTimer);
    pressTimer = null;
    pressing = false;
  }
  function click(e) {
    if (longPressed) {
      longPressed = false;
      return;
    }
    dispatch('open', { ctrlKey: e.ctrlKey, metaKey: e.metaKey, shiftKey: e.shiftKey });
  }
  function contextMenu(e) {
    // A finger held down raises a context menu too; the long press has
    // already answered it.
    if (Date.now() - longPressedAt < 1000) {
      e.preventDefault();
      return;
    }
    openGameMenu(e, game);
  }
</script>

<button
  class="card tile {cover}"
  class:enter={enter >= 0}
  style={enter >= 0 ? `--i: ${Math.min(enter, 12)}` : undefined}
  class:selected={selecting && selected}
  class:pressing
  title={game.savePath}
  on:click={click}
  on:contextmenu={contextMenu}
  on:pointerdown={pointerDown}
  on:pointermove={pointerMove}
  on:pointerup={endPress}
  on:pointercancel={endPress}
  on:mouseenter={() => (revealed = true)}
  on:mouseleave={() => {
    revealed = false;
    endPress();
  }}
>
  {#if selecting}
    <div class="tick" class:on={selected}>{#if selected}<Check size={14} strokeWidth={3} />{/if}</div>
  {/if}
  <div class="art" class:letterbox style="aspect-ratio: {COVER_STYLES[cover].aspect}">
    <!-- The art's layers, together, so they can be drawn a touch closer on
         hover without touching the cover's own transform (CoverImage uses
         one to hide a blur's soft edge). -->
    <div class="zoom">
      {#if backdrop}
        <div class="backdrop" style="background-image: url('{backdrop}')"></div>
      {/if}
      {#if src && isDaemonURL(src)}
        <CoverImage {src} {revealed} on:load={measure} />
      {:else if src}
        <img {src} alt="" loading="lazy" on:load={measure} on:error={(e) => (e.currentTarget.style.display = 'none')} />
      {/if}
      <div class="fallback"><span>{game.name}</span></div>
    </div>
  </div>
  <div class="body">
    <div class="name">
      {#if isFavourite($collections, game.id)}<span class="fav" title="In Favourites"><Star size={12} /></span>{/if}{game.name}
    </div>
    <div class="status tone-{status.tone}">
      {#if status.tone === 'warn'}<TriangleAlert size={13} class="status-icon" />{:else if status.tone === 'busy'}<RefreshCw
          size={12}
          class="status-icon spin"
        />{/if}<span class="status-text" title={status.label}>{status.label}</span>
    </div>
    {#if cover === 'wide'}
      <div class="meta">
        {snapshots} {snapshots === 1 ? 'snapshot' : 'snapshots'}{#if game.playtimeMs >= 60_000}{' · '}{playLength(game.playtimeMs)} played{/if}
        {#if game.activeBranch && game.activeBranch !== 'main'}
          · on <strong>{game.activeBranch}</strong>
        {/if}
      </div>
    {/if}
  </div>
</button>

<style>
  .tile {
    position: relative;
    display: flex;
    flex-direction: column;
    text-align: left;
    cursor: pointer;
    color: var(--text);
    transition:
      border-color 0.15s ease,
      box-shadow 0.22s ease,
      transform 0.22s cubic-bezier(0.2, 0.7, 0.2, 1);
    padding: 0;
    overflow: hidden;
    min-width: 0;
  }
  /* Pointed at: it stands out — a firmer edge and a shadow beneath — and,
     with animations on, rises and draws its art a little closer. */
  .tile:hover,
  .tile:focus-visible {
    border-color: rgba(var(--accent-rgb), 0.45);
    box-shadow: var(--shadow-lift);
  }
  :global(html[data-motion='on']) .tile:hover,
  :global(html[data-motion='on']) .tile:global([data-menu-open]) {
    transform: translateY(-4px);
  }
  :global(html[data-motion='on']) .tile:hover .zoom,
  :global(html[data-motion='on']) .tile:global([data-menu-open]) .zoom {
    transform: scale(1.04);
  }
  /* Its menu is open (lib/contextmenu.js): held up and outlined, so it is
     plain which game the menu is for once the pointer has moved onto it. */
  .tile:global([data-menu-open]) {
    border-color: var(--accent);
    box-shadow:
      0 0 0 1px var(--accent),
      var(--shadow-lift);
  }
  .tile.selected {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px var(--accent);
  }
  /* Held down: it gives a little as the press becomes a long one, so the
     gesture shows itself. The scale property, not transform, which the
     hover lift already uses. */
  :global(html[data-motion='on']) .tile.pressing {
    scale: 0.97;
    transition:
      scale 0.45s ease-out,
      border-color 0.15s ease,
      box-shadow 0.22s ease,
      transform 0.22s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  /* Backwards, not both: once in, the tile's own hover lift must not be
     held down by the animation's last frame. */
  .tile.enter {
    animation: tile-in 0.34s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 32ms);
  }
  @keyframes tile-in {
    from {
      opacity: 0;
      transform: translateY(10px) scale(0.985);
    }
  }
  .tick {
    position: absolute;
    top: 8px;
    left: 8px;
    z-index: 4;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: rgba(0, 0, 0, 0.55);
    border: 2px solid #fff;
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.8rem;
    font-weight: 700;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
  }
  .tick.on {
    background: var(--accent);
    border-color: var(--accent);
  }
  .art {
    position: relative;
    width: 100%;
    background: var(--bg);
    border-bottom: 1px solid var(--border);
    overflow: hidden;
  }
  .zoom {
    position: absolute;
    inset: 0;
    transition: transform 0.35s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .art :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    z-index: 2;
  }
  .art.letterbox :global(img) {
    object-fit: contain;
  }
  .backdrop {
    position: absolute;
    inset: -12px;
    z-index: 1;
    background-size: cover;
    background-position: center;
    filter: blur(14px) brightness(0.55) saturate(1.1);
  }
  .fallback {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 12px;
    background: linear-gradient(160deg, rgba(var(--accent-rgb), 0.2), rgba(var(--accent-rgb), 0.04));
  }
  .fallback span {
    font-weight: 700;
    font-size: 1rem;
    color: var(--text-dim);
    text-align: center;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
  }
  .body {
    padding: 11px 14px 13px;
    min-width: 0;
  }
  .tall .body {
    padding: 9px 11px 11px;
  }
  .name {
    font-weight: 600;
    margin-bottom: 5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .fav {
    display: inline-flex;
    vertical-align: -1px;
    margin-right: 5px;
    color: var(--warn);
  }
  .fav :global(svg) {
    fill: currentColor;
  }
  .tall .name {
    font-size: 0.88rem;
    margin-bottom: 3px;
  }
  /* Where the save stands. Quiet when all is well — a green dot on every
     card said nothing the summary above had not — and coloured, with an
     icon, only when it is worth a look. */
  .status {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 0.8rem;
    color: var(--text-dim);
    min-width: 0;
  }
  .tall .status {
    font-size: 0.75rem;
    gap: 6px;
  }
  .status-text {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .status.tone-warn {
    color: var(--warn);
  }
  .status.tone-busy {
    color: var(--accent);
  }
  .status.tone-muted {
    color: var(--text-faint);
  }
  .status :global(.status-icon) {
    flex-shrink: 0;
  }
  .status :global(.spin) {
    animation: tile-spin 1.6s linear infinite;
  }
  @keyframes tile-spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .status :global(.spin) {
      animation: none;
    }
  }
  .meta {
    margin-top: 4px;
    font-size: 0.75rem;
    color: var(--text-faint);
  }
</style>
