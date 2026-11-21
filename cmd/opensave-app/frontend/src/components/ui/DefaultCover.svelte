<script>
  // A game's picture when it has no cover art: a colour of its own
  // (lib/defaultcover.js), a controller, and its name. Fills its parent, and
  // sits under the CoverImage there, which covers it whenever art arrives.
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import { coverHue } from '../../lib/defaultcover.js';

  export let name = '';

  $: hue = coverHue(name);
</script>

<span class="default" style="--h: {hue}" aria-hidden="true">
  <span class="mark"><Gamepad2 size="100%" strokeWidth={1.4} /></span>
  <span class="name">{name}</span>
</span>

<style>
  .default {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
    overflow: hidden;
    background:
      radial-gradient(120% 70% at 15% 0%, hsl(var(--h) 55% 45% / 0.55), transparent 60%),
      linear-gradient(160deg, hsl(var(--h) 38% 26%), hsl(calc(var(--h) + 40) 42% 11%));
    container-type: inline-size;
  }
  .mark {
    position: absolute;
    top: 22%;
    left: 50%;
    width: 46%;
    aspect-ratio: 1;
    transform: translateX(-50%);
    color: hsl(var(--h) 60% 85% / 0.28);
  }
  .mark :global(svg) {
    display: block;
    width: 100%;
    height: 100%;
  }
  .name {
    position: relative;
    padding: 9%;
    color: #fff;
    font-size: clamp(0.62rem, 11cqi, 0.95rem);
    font-weight: 700;
    line-height: 1.2;
    text-align: left;
    text-shadow: 0 1px 3px rgba(0, 0, 0, 0.5);
    overflow-wrap: anywhere;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  /* Too small for words to be read: the colour and the controller alone. */
  @container (max-width: 64px) {
    .name {
      display: none;
    }
    .mark {
      top: 50%;
      width: 60%;
      transform: translate(-50%, -50%);
    }
  }
</style>
