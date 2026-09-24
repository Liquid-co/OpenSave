<script>
  // A portrait box-art tile: the scan results, the cloud browser and the
  // backup export all show games this way. It was written out three times, a
  // hundred and fifty lines of markup and style each, and the copies had begun
  // to differ in small ways nobody chose.
  //
  // Clicking it, or Enter or Space on it, fires `activate`. A dimmed tile (a
  // save already tracked, shown for reference) stays put.
  import { createEventDispatcher } from 'svelte';
  import CoverImage from '../CoverImage.svelte';
  import { isDaemonURL } from '../../lib/api.js';

  export let name = '';
  /** Tooltip for the whole tile; the name when not given. */
  export let title = '';
  /** Cover URL. Blank shows the fallback: the emoji over the name. */
  export let src = '';
  export let emoji = '🎮';
  export let selected = false;
  /** Small label at the top right: "Emulator", "Tracked", "☁ 3". */
  export let badge = '';
  export let badgeAccent = false;
  /** Pill at the bottom left, for a state worth seeing at a glance. */
  export let stamp = '';
  export let dimmed = false;
  /** Drawn faintly: there is something here, but nothing in it yet. */
  export let faded = false;
  /** One line under the name. */
  export let meta = '';
  export let metaTitle = '';
  /** How the hover slot's buttons sit: 'stack' fills the bottom, 'center' is one centred button. */
  export let hoverLayout = 'stack';

  const dispatch = createEventDispatcher();
  const activate = () => {
    if (!dimmed) dispatch('activate');
  };
  // Only a key pressed on the tile itself. The buttons inside it are focusable
  // too, and their Enter bubbles up here: taking it (and preventing its
  // default) made Enter on a focused Track button select the tile instead.
  const onKeydown = (e) => {
    if (e.target !== e.currentTarget) return;
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      activate();
    }
  };

  // Explicit covers are blurred until the pointer or focus is on this tile.
  // Only a cover fetched from the daemon can be checked for that — it is the
  // daemon that says so, in a header — so any other address (a custom cover
  // on some image host) is shown as a plain image, which is what it always was.
  let revealed = false;
</script>

<div
  class="tile"
  class:sel={selected}
  class:dimmed
  class:faded
  on:click={activate}
  on:keydown={onKeydown}
  on:mouseenter={() => (revealed = true)}
  on:mouseleave={() => (revealed = false)}
  on:focus={() => (revealed = true)}
  on:blur={() => (revealed = false)}
  role="button"
  tabindex="0"
  title={title || name}
>
  <div class="art">
    {#if src && isDaemonURL(src)}
      <CoverImage {src} alt={name} {revealed} />
    {:else if src}
      <img {src} alt={name} loading="lazy" on:error={(e) => (e.currentTarget.style.display = 'none')} />
    {/if}
    <div class="fallback">
      <span class="emoji">{emoji}</span>
      <span class="fallback-name">{name}</span>
    </div>

    {#if selected}
      <div class="tick">✓</div>
    {/if}
    {#if badge}
      <span class="corner" class:accent={badgeAccent}>{badge}</span>
    {/if}
    {#if stamp}
      <span class="stamp">{stamp}</span>
    {/if}
    {#if $$slots.hover && !dimmed}
      <div class="hover {hoverLayout}"><slot name="hover" /></div>
    {/if}
  </div>
  <div class="name" title={name}>{name}</div>
  {#if meta}
    <div class="meta" title={metaTitle || meta}>{meta}</div>
  {/if}
  <slot />
</div>

<style>
  .tile {
    cursor: pointer;
    outline: none;
  }
  .art {
    position: relative;
    aspect-ratio: 600 / 900;
    border-radius: 10px;
    overflow: hidden;
    background: var(--bg-active);
    border: 2px solid transparent;
    transition: transform 0.12s, border-color 0.12s, box-shadow 0.12s;
  }
  .tile:hover .art {
    transform: translateY(-2px);
    box-shadow: 0 8px 22px rgba(0, 0, 0, 0.45);
  }
  .tile.sel .art {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--accent-soft);
  }
  .art :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    z-index: 2;
  }
  /* A tile with no cover, or one whose image failed, shows this through: it
     sits underneath at a lower z-index. */
  .fallback {
    position: absolute;
    inset: 0;
    z-index: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 14px;
    text-align: center;
    background: linear-gradient(160deg, rgba(138, 99, 244, 0.22), rgba(138, 99, 244, 0.04));
  }
  .emoji {
    font-size: 2.2rem;
  }
  .fallback-name {
    font-weight: 700;
    font-size: 0.9rem;
    color: var(--text);
    line-height: 1.25;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .tick {
    position: absolute;
    top: 8px;
    left: 8px;
    z-index: 3;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: var(--accent);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.8rem;
    font-weight: 700;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
  }
  .corner {
    position: absolute;
    top: 8px;
    right: 8px;
    z-index: 3;
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 0.68rem;
    font-weight: 600;
    background: rgba(0, 0, 0, 0.6);
    color: var(--text-dim);
    backdrop-filter: blur(2px);
  }
  .corner.accent {
    color: var(--accent);
  }
  .stamp {
    position: absolute;
    left: 8px;
    bottom: 8px;
    z-index: 3;
    padding: 3px 9px;
    border-radius: 999px;
    font-size: 0.68rem;
    font-weight: 700;
    background: var(--accent);
    color: #fff;
  }
  .hover {
    position: absolute;
    inset: 0;
    z-index: 3;
    display: flex;
    padding: 12px;
    opacity: 0;
    background: linear-gradient(to top, rgba(0, 0, 0, 0.75), transparent 55%);
    transition: opacity 0.12s;
  }
  /* Stacked, not side by side: a cover tile is portrait and narrow, and two
     buttons in a row wrap raggedly at the smaller grid sizes. */
  .hover.stack {
    flex-direction: column;
    align-items: stretch;
    justify-content: flex-end;
    gap: 6px;
  }
  .hover.center {
    align-items: flex-end;
    justify-content: center;
  }
  .tile:hover .hover,
  /* The overlay only appears on hover, so a keyboard user tabbing to these
     buttons would otherwise be operating something invisible. */
  .hover:focus-within {
    opacity: 1;
  }
  .tile.dimmed {
    cursor: default;
    opacity: 0.6;
  }
  .tile.dimmed:hover .art {
    transform: none;
  }
  .tile.faded .art {
    opacity: 0.45;
  }
  .name {
    margin-top: 7px;
    font-size: 0.82rem;
    font-weight: 500;
    color: var(--text-dim);
    text-align: center;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .tile.sel .name {
    color: var(--text);
  }
  .meta {
    margin-top: 2px;
    font-size: 0.72rem;
    color: var(--text-faint);
    text-align: center;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .tile.faded .meta {
    font-style: italic;
  }
</style>
