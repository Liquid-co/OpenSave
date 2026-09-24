<script>
  // Theme, accent colour and size, for this device. Applied as they are
  // chosen; see lib/appearance.js.
  import Check from 'lucide-svelte/icons/check';
  import Moon from 'lucide-svelte/icons/moon';
  import Sun from 'lucide-svelte/icons/sun';
  import MonitorCog from 'lucide-svelte/icons/monitor-cog';
  import { appearance, THEMES, ACCENTS, SCALES } from '../../lib/appearance.js';

  const set = (patch) => appearance.update((v) => ({ ...v, ...patch }));
  const themeIcons = { dark: Moon, light: Sun, system: MonitorCog };
</script>

<div class="options">
  <div class="group">
    <span class="label" id="ap-theme">Theme</span>
    <div class="segmented" role="radiogroup" aria-labelledby="ap-theme">
      {#each Object.entries(THEMES) as [id, label]}
        <button role="radio" aria-checked={$appearance.theme === id} class:on={$appearance.theme === id} on:click={() => set({ theme: id })}>
          <svelte:component this={themeIcons[id]} size={14} />{label}
        </button>
      {/each}
    </div>
  </div>

  <div class="group">
    <span class="label" id="ap-accent">Accent colour</span>
    <div class="swatches" role="radiogroup" aria-labelledby="ap-accent">
      {#each Object.entries(ACCENTS) as [id, a]}
        <button
          class="swatch"
          role="radio"
          aria-checked={$appearance.accent === id}
          aria-label={a.label}
          title={a.label}
          style="--swatch: {a.hex}"
          on:click={() => set({ accent: id })}
        >
          {#if $appearance.accent === id}<Check size={14} strokeWidth={3} />{/if}
        </button>
      {/each}
    </div>
  </div>

  <label class="group">
    <span class="label">Size</span>
    <select value={String($appearance.scale)} on:change={(e) => set({ scale: Number(e.currentTarget.value) })}>
      {#each SCALES as s}
        <option value={String(s)}>{Math.round(s * 100)}%{s === 1 ? ' (default)' : ''}</option>
      {/each}
    </select>
  </label>
</div>

<style>
  .options {
    display: flex;
    flex-wrap: wrap;
    gap: 18px 28px;
    align-items: flex-end;
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
  .segmented {
    display: inline-flex;
    border: 1px solid var(--border-strong);
    border-radius: 9px;
    overflow: hidden;
  }
  .segmented button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 7px 13px;
    background: var(--bg);
    border: none;
    border-left: 1px solid var(--border-strong);
    color: var(--text-dim);
    font: inherit;
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
  .swatches {
    display: flex;
    gap: 8px;
    padding: 3px 0;
  }
  .swatch {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    border: none;
    background: var(--swatch);
    color: #fff;
    display: grid;
    place-items: center;
    cursor: pointer;
    box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.15);
    transition: transform 0.12s;
  }
  .swatch:hover {
    transform: scale(1.08);
  }
  .swatch[aria-checked='true'] {
    box-shadow:
      0 0 0 2px var(--bg-raised),
      0 0 0 4px var(--swatch);
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
</style>
