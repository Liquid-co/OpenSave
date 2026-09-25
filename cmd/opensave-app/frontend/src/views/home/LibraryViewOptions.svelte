<script>
  // How the library looks: cover style, tiles per row, tile size. Shown in
  // Settings and in the library's View menu; both change the same thing, at
  // once. See lib/libraryview.js.
  import { libraryView, COVER_STYLES, TILE_SIZES, COLUMN_CHOICES } from '../../lib/libraryview.js';

  const set = (patch) => libraryView.update((v) => ({ ...v, ...patch }));
</script>

<div class="options">
  <div class="group">
    <div class="label">Covers</div>
    <div class="styles" role="radiogroup" aria-label="Cover style">
      {#each Object.entries(COVER_STYLES) as [id, style]}
        <button
          class="style"
          class:on={$libraryView.cover === id}
          role="radio"
          aria-checked={$libraryView.cover === id}
          on:click={() => set({ cover: id })}
        >
          <span class="mini {id}" aria-hidden="true">
            {#each Array(id === 'wide' ? 3 : 5) as _}<span></span>{/each}
          </span>
          <span class="style-name">{style.label}</span>
        </button>
      {/each}
    </div>
  </div>

  <div class="row">
    <label class="group">
      <span class="label">Tiles per row</span>
      <select value={String($libraryView.columns)} on:change={(e) => set({ columns: e.currentTarget.value === 'auto' ? 'auto' : Number(e.currentTarget.value) })}>
        {#each COLUMN_CHOICES as c}
          <option value={String(c)}>{c === 'auto' ? 'As many as fit' : c}</option>
        {/each}
      </select>
    </label>

    <div class="group">
      <span class="label">Tile size</span>
      <div class="segmented" class:off={$libraryView.columns !== 'auto'}>
        {#each Object.entries(TILE_SIZES) as [id, label]}
          <button
            class:on={$libraryView.size === id}
            disabled={$libraryView.columns !== 'auto'}
            on:click={() => set({ size: id })}
          >
            {label}
          </button>
        {/each}
      </div>
    </div>
  </div>
  {#if $libraryView.columns !== 'auto'}
    <p class="note">With a set number per row, the tiles share the width between them.</p>
  {/if}

  <label class="check">
    <input type="checkbox" checked={$libraryView.overview} on:change={(e) => set({ overview: e.currentTarget.checked })} />
    Show recent activity and the last 14 days above the library
  </label>
</div>

<style>
  .options {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .group {
    display: flex;
    flex-direction: column;
    gap: 7px;
  }
  .label {
    font-size: 0.82rem;
    font-weight: 600;
    color: var(--text-dim);
  }
  .styles {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }
  .style {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: 12px 10px 10px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--text-dim);
    cursor: pointer;
    transition: border-color 0.12s, background 0.12s;
  }
  .style:hover {
    border-color: var(--border-strong);
  }
  .style.on {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--text);
  }
  .style-name {
    font-size: 0.85rem;
    font-weight: 600;
  }
  /* A sketch of the shelf each style makes, both in one height so the two
     labels line up. */
  .mini {
    display: grid;
    gap: 4px;
    width: 100%;
    max-width: 150px;
    height: 42px;
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
    background: linear-gradient(160deg, rgba(var(--accent-rgb), 0.55), rgba(var(--accent-rgb), 0.18));
  }
  .mini.wide span {
    aspect-ratio: 460 / 215;
  }
  .mini.tall span {
    aspect-ratio: 600 / 900;
  }
  .row {
    display: flex;
    gap: 18px;
    flex-wrap: wrap;
  }
  select {
    padding: 7px 10px;
    background-color: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.88rem;
    min-width: 150px;
  }
  .segmented {
    display: inline-flex;
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    overflow: hidden;
  }
  .segmented button {
    padding: 7px 14px;
    background: var(--bg);
    border: none;
    border-left: 1px solid var(--border-strong);
    color: var(--text-dim);
    font-size: 0.85rem;
    cursor: pointer;
  }
  .segmented button:first-child {
    border-left: none;
  }
  .segmented button.on {
    background: var(--accent-soft);
    color: var(--text);
    font-weight: 600;
  }
  .segmented.off {
    opacity: 0.5;
  }
  .segmented button:disabled {
    cursor: default;
  }
  .note {
    font-size: 0.78rem;
    color: var(--text-faint);
    margin-top: -6px;
  }
</style>
