<script>
  // How the library looks: cover style, tiles per row, tile size, and the
  // Recently played row. Shown in Settings and in the library's View menu;
  // both change the same thing, at once. See lib/libraryview.js.
  //
  // Every choice is a row of buttons rather than a list box, so what can be
  // picked is in view, and one marker slides to the one picked.
  import Check from 'lucide-svelte/icons/check';
  import { t } from '../../lib/i18n.js';
  import { slidingIndicator } from '../../lib/motion.js';
  import { libraryView, COVER_STYLES, TILE_SIZES, COLUMN_CHOICES } from '../../lib/libraryview.js';

  const set = (patch) => libraryView.update((v) => ({ ...v, ...patch }));
  $: fixed = $libraryView.columns !== 'auto';
</script>

<div class="options">
  <div class="group">
    <span class="label" id="lv-covers">{$t('home.viewOptions.covers')}</span>
    <div class="styles" role="radiogroup" aria-labelledby="lv-covers">
      {#each Object.entries(COVER_STYLES) as [id]}
        <button class="style" class:on={$libraryView.cover === id} role="radio" aria-checked={$libraryView.cover === id} on:click={() => set({ cover: id })}>
          <span class="mini {id}" aria-hidden="true">
            {#each Array(id === 'wide' ? 3 : 5) as _, i}<span style="--i: {i}"></span>{/each}
          </span>
          <span class="style-name">{$t('libraryView.cover.' + id)}</span>
          {#if $libraryView.cover === id}<span class="tick"><Check size={12} strokeWidth={3} /></span>{/if}
        </button>
      {/each}
    </div>
  </div>

  <div class="group">
    <span class="label" id="lv-columns">{$t('home.viewOptions.tilesPerRow')}</span>
    <div class="seg" role="radiogroup" aria-labelledby="lv-columns" use:slidingIndicator={{ mode: 'fill', className: 'seg-mark' }}>
      {#each COLUMN_CHOICES as c}
        <button
          class:on={$libraryView.columns === c}
          class:wide={c === 'auto'}
          role="radio"
          aria-checked={$libraryView.columns === c}
          title={c === 'auto' ? $t('home.viewOptions.autoFit') : ''}
          on:click={() => set({ columns: c })}
        >
          {c === 'auto' ? $t('home.viewOptions.auto') : c}
        </button>
      {/each}
    </div>
  </div>

  <div class="group" class:off={fixed}>
    <span class="label" id="lv-size">{$t('home.viewOptions.tileSize')}</span>
    <div class="seg" role="radiogroup" aria-labelledby="lv-size" use:slidingIndicator={{ mode: 'fill', className: 'seg-mark' }}>
      {#each Object.keys(TILE_SIZES) as id}
        <button class:on={$libraryView.size === id} class:wide={true} role="radio" aria-checked={$libraryView.size === id} disabled={fixed} on:click={() => set({ size: id })}>
          {$t('libraryView.tile.' + id)}
        </button>
      {/each}
    </div>
    {#if fixed}<p class="note">{$t('home.viewOptions.fixedNote')}</p>{/if}
  </div>

  <label class="check">
    <input type="checkbox" checked={$libraryView.overview} on:change={(e) => set({ overview: e.currentTarget.checked })} />
    {$t('home.viewOptions.overview')}
  </label>
</div>

<style>
  .options {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  .group {
    display: flex;
    flex-direction: column;
    gap: 8px;
    transition: opacity 0.15s;
  }
  .group.off {
    opacity: 0.55;
  }
  .label {
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }

  /* The two cover styles, each a sketch of the shelf it makes. */
  .styles {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }
  .style {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 14px 10px 12px;
    background: var(--bg);
    border: 1.5px solid var(--border);
    border-radius: 12px;
    color: var(--text-dim);
    font: inherit;
    cursor: pointer;
    transition:
      border-color 0.15s,
      background 0.15s,
      color 0.15s,
      transform 0.16s cubic-bezier(0.2, 0.7, 0.2, 1.4);
  }
  .style:hover {
    border-color: var(--border-strong);
    color: var(--text);
  }
  .style:active {
    transform: scale(0.97);
  }
  .style.on {
    border-color: var(--accent);
    background: linear-gradient(0deg, var(--accent-soft), var(--accent-soft)), var(--bg);
    color: var(--text);
  }
  .tick {
    position: absolute;
    top: 7px;
    right: 7px;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: var(--accent-fill);
    color: var(--on-accent);
    animation: tick-in 0.2s cubic-bezier(0.2, 0.7, 0.2, 1.4) backwards;
  }
  @keyframes tick-in {
    from {
      transform: scale(0.4);
      opacity: 0;
    }
  }
  .style-name {
    font-size: var(--text-md);
    font-weight: 600;
  }
  .mini {
    display: grid;
    gap: 4px;
    width: 100%;
    max-width: 150px;
    height: 44px;
    align-content: center;
  }
  .mini.wide {
    grid-template-columns: repeat(3, 1fr);
  }
  .mini.tall {
    grid-template-columns: repeat(5, 1fr);
  }
  .mini span {
    border-radius: 3px;
    background: linear-gradient(160deg, rgba(var(--accent-rgb), 0.5), rgba(var(--accent-rgb), 0.15));
    transition: transform 0.25s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  /* Hovered, the sketch's covers lift one after another. */
  .style:hover .mini span {
    transform: translateY(-2px);
    transition-delay: calc(var(--i) * 30ms);
  }
  .mini.wide span {
    aspect-ratio: 460 / 215;
  }
  .mini.tall span {
    aspect-ratio: 600 / 900;
  }

  /* A row of choices on a track, one marker sliding to the one picked. */
  .seg {
    display: flex;
    padding: 3px;
    gap: 2px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 10px;
  }
  .seg button {
    flex: 1;
    min-width: 0;
    height: 30px;
    padding: 0 6px;
    border: 1px solid transparent;
    border-radius: 7px;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-md);
    cursor: pointer;
    transition: color 0.12s;
  }
  .seg button.wide {
    flex: 1.8;
  }
  .seg button:hover:not(:disabled) {
    color: var(--text);
  }
  .seg button.on {
    background: var(--bg-active);
    border-color: var(--border-strong);
    color: var(--text);
    font-weight: 600;
  }
  .seg:global(.slides) button {
    position: relative;
    z-index: 1;
  }
  .seg:global(.slides) button.on {
    background: transparent;
    border-color: transparent;
  }
  .seg :global(.seg-mark) {
    background: var(--bg-active);
    border: 1px solid var(--border-strong);
    border-radius: 7px;
  }
  .seg button:disabled {
    cursor: default;
  }
  .note {
    font-size: var(--text-sm);
    color: var(--text-faint);
    line-height: 1.45;
  }
  .check {
    font-size: var(--text-base);
  }
</style>
