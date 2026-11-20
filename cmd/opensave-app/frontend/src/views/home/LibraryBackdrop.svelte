<script>
  // The light at the top of the Library: the cover of the game being played,
  // or played last, blurred out of shape, its colour lifted, darkened so text
  // stays readable over it, with a little grain, fading into the page. Or a
  // glow in the accent colour, or nothing (Settings → Appearance). It changes
  // with the game, and cross-fades when it does.
  //
  // Not a CSS blur. blur(60px) over an area this size stopped a window
  // without GPU help drawing at all — no frame for seconds. Instead the cover
  // is painted once into a canvas a few dozen pixels across and blurred there,
  // which costs next to nothing, and the canvas is stretched to fill: an image
  // that small, scaled up, is a blur already.
  //
  // Art marked explicit is never used, blurred or not: the glow stands in.
  import { fade } from 'svelte/transition';
  import { appearance } from '../../lib/appearance.js';
  import { gameCover } from '../../lib/api.js';

  /** The game to take the cover from, or null. */
  export let game = null;

  // The shown canvas's own size; the stretch to the page does the rest.
  const W = 160;
  const H = 72;

  let failed = '';
  $: mode = $appearance.backdrop;
  $: src = mode === 'cover' && game && !game.coverExplicit ? gameCover(game, false) : '';
  $: still = $appearance.motion === false;
  $: showCover = !!src && failed !== src;

  // Each canvas paints its own cover once it is on the page; a new game is a
  // new canvas (keyed below), so an old one is never painted over.
  function painted(node, url) {
    const img = new Image();
    img.onload = () => {
      if (!node.isConnected) return;
      // First into a tiny canvas, blurred and coloured there (cheap at this
      // size); then up into the shown one with smoothing, for soft edges.
      const tiny = document.createElement('canvas');
      tiny.width = 40;
      tiny.height = 18;
      const t = tiny.getContext('2d');
      t.filter = 'blur(1.5px) saturate(1.35) brightness(0.8)';
      t.drawImage(img, -4, -3, 48, 24);
      node.width = W;
      node.height = H;
      const c = node.getContext('2d');
      c.imageSmoothingEnabled = true;
      c.imageSmoothingQuality = 'high';
      c.filter = 'blur(4px)';
      c.drawImage(tiny, -8, -6, W + 16, H + 12);
      node.classList.add('ready');
    };
    img.onerror = () => (failed = url);
    img.src = url;
  }
</script>

{#if mode !== 'off'}
  <div class="backdrop" aria-hidden="true">
    {#if showCover}
      {#key src}
        <canvas use:painted={src} transition:fade={{ duration: still ? 0 : 600 }}></canvas>
      {/key}
      <div class="grain"></div>
    {:else}
      <div class="glow"></div>
    {/if}
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
  canvas {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    opacity: 0;
  }
  canvas:global(.ready) {
    opacity: 0.62;
  }
  :global([data-theme='light']) canvas:global(.ready) {
    opacity: 0.3;
  }
  .grain {
    position: absolute;
    inset: 0;
    opacity: 0.05;
    background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='180' height='180'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' stitchTiles='stitch'/></filter><rect width='100%' height='100%' filter='url(%23n)'/></svg>");
  }
  /* The glow is drawn with gradients, not a blur, for the same reason. */
  .glow {
    position: absolute;
    inset: 0;
    opacity: 0.5;
    background:
      radial-gradient(45% 70% at 22% 10%, rgba(var(--accent-rgb), 0.45), transparent 70%),
      radial-gradient(40% 65% at 78% 15%, rgba(var(--accent-rgb), 0.25), transparent 70%);
  }
  :global([data-theme='light']) .glow {
    opacity: 0.35;
  }
  .fade {
    position: absolute;
    inset: 0;
    background:
      radial-gradient(130% 100% at 50% 0%, transparent 35%, var(--bg) 100%),
      linear-gradient(180deg, transparent 25%, var(--bg) 100%);
  }
</style>
