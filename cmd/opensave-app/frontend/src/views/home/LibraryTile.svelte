<script>
  // One game in the library, as a wide banner or as tall box art.
  //
  // Tall art is not there for every game: Steam has a banner for everything
  // but a library capsule only for some, and a custom cover is whatever shape
  // someone found. Art whose shape is far from the tile's is shown whole over
  // a blurred copy of itself rather than cropped to a sliver of its middle.
  import { createEventDispatcher } from 'svelte';
  import CoverImage from '../../components/CoverImage.svelte';
  import { coverURL, gameCover, isDaemonURL } from '../../lib/api.js';
  import { COVER_STYLES } from '../../lib/libraryview.js';

  export let game;
  export let status;
  /** 'wide' or 'tall'. */
  export let cover = 'wide';
  export let selecting = false;
  export let selected = false;

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
</script>

<button
  class="card tile {cover}"
  class:selected={selecting && selected}
  title={game.savePath}
  on:click={() => dispatch('open')}
  on:mouseenter={() => (revealed = true)}
  on:mouseleave={() => (revealed = false)}
>
  {#if selecting}
    <div class="tick" class:on={selected}>{selected ? '✓' : ''}</div>
  {/if}
  <div class="art" class:letterbox style="aspect-ratio: {COVER_STYLES[cover].aspect}">
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
  <div class="body">
    <div class="name">{game.name}</div>
    <div class="status tone-{status.tone}">
      <span class="dot"></span><span class="status-text" title={status.label}>{status.label}</span>
    </div>
    {#if cover === 'wide'}
      <div class="meta">
        {snapshots} {snapshots === 1 ? 'snapshot' : 'snapshots'}
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
    transition: border-color 0.12s, transform 0.12s;
    padding: 0;
    overflow: hidden;
    min-width: 0;
  }
  .tile:hover {
    border-color: var(--border-strong);
    transform: translateY(-1px);
  }
  .tile.selected {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px var(--accent);
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
    background: linear-gradient(160deg, rgba(138, 99, 244, 0.2), rgba(138, 99, 244, 0.04));
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
  .tall .name {
    font-size: 0.88rem;
    margin-bottom: 3px;
  }
  /* Where the save stands, in the colour that says whether to look. */
  .status {
    --tone: var(--success);
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 0.8rem;
    color: var(--text);
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
    --tone: var(--warn);
    color: var(--warn);
  }
  .status.tone-busy {
    --tone: var(--accent);
    color: var(--accent);
  }
  .status.tone-muted {
    --tone: var(--text-faint);
    color: var(--text-dim);
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--tone);
    flex-shrink: 0;
  }
  .meta {
    margin-top: 4px;
    font-size: 0.75rem;
    color: var(--text-faint);
  }
</style>
