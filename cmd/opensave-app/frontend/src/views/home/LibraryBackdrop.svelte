<script>
  // The light at the top of the Library: a game's cover, softened, its colour
  // lifted, darkened so text stays readable over it, with a little grain,
  // fading into the page. It starts on the game being played, or played last,
  // and — unless that is switched off (Settings → Appearance) — moves on
  // through the rest of the library, each cover cross-fading into the next.
  // Or nothing at all.
  //
  // Not a CSS blur. A large blur over an area this size stopped a window
  // without GPU help drawing at all — no frame for seconds. Instead each cover
  // is painted once into a canvas a few hundred pixels across, softened there,
  // which costs next to nothing, and the canvas is stretched to fill.
  //
  // The cross-fade has no dip and no flash: the covers sit at full strength
  // inside one layer that carries the dimming, the next fades in on top of
  // the one before, and only then is the old one taken away. A cover is
  // loaded before it is shown, so a slow or missing one never leaves a gap.
  //
  // Art marked explicit is never used, blurred or not.
  import { onDestroy, onMount } from 'svelte';
  import { appearance } from '../../lib/appearance.js';
  import { motionOn } from '../../lib/motion.js';
  import { gameCover } from '../../lib/api.js';
  import { backdropCycle } from '../../lib/libraryhome.js';

  /** The game to start on (being played, or played last), or null. */
  export let game = null;
  /** The library, for moving on through. */
  export let games = [];
  /** How far down the page it reaches, in pixels. */
  export let height = 460;

  // The canvas's own size; the stretch to the page does the rest. Big enough
  // that a cover is still recognisable, small enough to paint in a moment.
  const W = 480;
  const H = 216;
  const BLUR = 3;
  // How long each cover stays, and how long one takes to become the next.
  const HOLD = 12_000;
  const FADE = 1_600;

  const urlOf = (g) => (g && !g.coverExplicit ? gameCover(g, false) : '');

  $: mode = $appearance.backdrop;
  $: order = backdropCycle(games, game);
  $: cycling = mode === 'cover' && $appearance.backdropCycle && $motionOn && order.length > 1;

  // The cover up now, and covers that would not load (not tried again).
  let shown = null;
  const bad = new Set();
  // Each new canvas goes on top of the one before.
  let layer = 0;
  $: src = urlOf(shown);

  let timer;
  let ticket = 0;
  const preload = (url) =>
    new Promise((done) => {
      const img = new Image();
      img.onload = () => done(true);
      img.onerror = () => done(false);
      img.src = url;
    });

  /** Shows the first of `list` whose cover loads, then waits for the next. */
  async function showFirst(list) {
    const mine = ++ticket;
    clearTimeout(timer);
    for (const g of list) {
      const url = urlOf(g);
      if (!url || bad.has(url)) continue;
      if (url === src) {
        shown = g;
        wait();
        return;
      }
      const ok = await preload(url);
      if (mine !== ticket) return;
      if (ok) {
        shown = g;
        wait();
        return;
      }
      bad.add(url);
    }
    if (mine === ticket && !list.some((g) => urlOf(g) === src)) shown = null;
  }

  function wait() {
    clearTimeout(timer);
    if (cycling && !document.hidden) timer = setTimeout(next, HOLD);
  }

  function next() {
    const i = order.findIndex((g) => g.id === shown?.id);
    showFirst([...order.slice(i + 1), ...order.slice(0, Math.max(i, 0))]);
  }

  // A new game played, the setting changed, or the library's first game
  // arriving: start again from the game played last.
  $: startKey = `${mode}|${game?.id ?? ''}|${cycling}|${order[0]?.id ?? ''}`;
  $: startKey, start();
  function start() {
    if (mode !== 'cover') {
      ticket++;
      clearTimeout(timer);
      shown = null;
      return;
    }
    showFirst(cycling ? (game ? [game, ...order] : order) : game ? [game] : []);
  }

  // Not moving on while the window is hidden; carrying on when it is back.
  const visibility = () => (document.hidden ? clearTimeout(timer) : wait());
  onMount(() => document.addEventListener('visibilitychange', visibility));
  onDestroy(() => {
    ticket++;
    clearTimeout(timer);
    document.removeEventListener('visibilitychange', visibility);
  });

  // Each canvas paints its own cover once it is on the page; a new cover is a
  // new canvas (keyed below), so one is never painted over.
  function painted(node, url) {
    node.style.zIndex = String(++layer);
    const img = new Image();
    img.onload = () => {
      if (!node.isConnected) return;
      node.width = W;
      node.height = H;
      const c = node.getContext('2d');
      // Cover the canvas, and a little past it, so the soft edge of the blur
      // falls outside rather than darkening the border.
      const pad = BLUR * 3;
      const s = Math.max((W + pad * 2) / img.naturalWidth, (H + pad * 2) / img.naturalHeight);
      const w = img.naturalWidth * s;
      const h = img.naturalHeight * s;
      c.imageSmoothingEnabled = true;
      c.imageSmoothingQuality = 'high';
      c.filter = `blur(${BLUR}px) saturate(1.25) brightness(0.8)`;
      c.drawImage(img, (W - w) / 2, (H - h) / 2, w, h);
      requestAnimationFrame(() => node.classList.add('ready'));
    };
    img.onerror = () => bad.add(url);
    img.src = url;
  }

  // The cover going away stays, unchanged, under the one arriving until that
  // one is fully in.
  const stay = (node, { duration }) => ({ duration });
</script>

{#if mode === 'cover'}
  <div class="backdrop" aria-hidden="true" style="height: {height}px; --fade: {$motionOn ? FADE : 0}ms">
    <div class="art">
      <!-- Directly in the key block, so its outro runs when the cover changes
           (transitions are local to the block that holds them). -->
      {#if src}
        {#key src}
          <canvas use:painted={src} out:stay={{ duration: $motionOn ? FADE : 0 }}></canvas>
        {/key}
      {/if}
    </div>
    {#if src}<div class="grain"></div>{/if}
    <div class="fade"></div>
  </div>
{/if}

<style>
  .backdrop {
    position: absolute;
    /* Out to the page's edges, past main's padding (App.svelte). */
    top: -24px;
    left: -28px;
    right: -28px;
    height: 460px;
    overflow: hidden;
    pointer-events: none;
    z-index: -1;
  }
  /* The dimming is the layer's, not each cover's, so two covers mid-fade
     add up to exactly one. */
  .art {
    position: absolute;
    inset: 0;
    opacity: 0.6;
  }
  :global([data-theme='light']) .art {
    opacity: 0.3;
  }
  canvas {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    opacity: 0;
    transition: opacity var(--fade) ease;
  }
  canvas:global(.ready) {
    opacity: 1;
  }
  .grain {
    position: absolute;
    inset: 0;
    opacity: 0.05;
    background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='180' height='180'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' stitchTiles='stitch'/></filter><rect width='100%' height='100%' filter='url(%23n)'/></svg>");
  }
  .fade {
    position: absolute;
    inset: 0;
    background:
      radial-gradient(130% 100% at 50% 0%, transparent 35%, var(--bg) 100%),
      linear-gradient(180deg, transparent 25%, var(--bg) 100%);
  }
</style>
