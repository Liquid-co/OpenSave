<script>
  // Theme, accent colour, size and motion, for this device. Applied as they
  // are chosen; see lib/appearance.js.
  import Check from 'lucide-svelte/icons/check';
  import Moon from 'lucide-svelte/icons/moon';
  import Sun from 'lucide-svelte/icons/sun';
  import MonitorCog from 'lucide-svelte/icons/monitor-cog';
  import Plus from 'lucide-svelte/icons/plus';
  import X from 'lucide-svelte/icons/x';
  import {
    appearance,
    THEMES,
    ACCENTS,
    SCALES,
    onAccent,
    accentHex,
    withCustomAccent,
    withoutCustomAccent
  } from '../../lib/appearance.js';
  import { systemReducesMotion, slidingIndicator } from '../../lib/motion.js';
  import { locale, LOCALES, t, SYSTEM_LOCALE } from '../../lib/i18n.js';
  import AccentWheel from './AccentWheel.svelte';

  const set = (patch) => appearance.update((v) => ({ ...v, ...patch }));
  const themeIcons = { dark: Moon, light: Sun, system: MonitorCog };

  // The colour wheel, open beside its button until a colour is added or it is
  // dismissed: Escape, or a click anywhere else.
  let wheelOpen = false;
  let pickerEl;
  const addColour = (e) => {
    appearance.update((v) => withCustomAccent(v, e.detail));
    wheelOpen = false;
  };
  const removeColour = (hex) => appearance.update((v) => withoutCustomAccent(v, hex));
  function outside(e) {
    if (wheelOpen && pickerEl && !pickerEl.contains(e.target)) wheelOpen = false;
  }
</script>

<svelte:window on:pointerdown={outside} on:keydown={(e) => wheelOpen && e.key === 'Escape' && (wheelOpen = false)} />

<div class="options">
  <label class="group">
    <span class="label">{$t('settings.language')}</span>
    <select value={$locale} on:change={(e) => locale.set(e.currentTarget.value)}>
      <option value={SYSTEM_LOCALE}>{$t('settings.systemDefault')}</option>
      {#each Object.entries(LOCALES) as [id, label]}
        <option value={id}>{label}</option>
      {/each}
    </select>
  </label>

  <div class="group">
    <span class="label" id="ap-theme">{$t('settings.appearance.theme')}</span>
    <div class="segmented" role="radiogroup" aria-labelledby="ap-theme" use:slidingIndicator={{ mode: 'fill' }}>
      {#each Object.entries(THEMES) as [id]}
        <button role="radio" aria-checked={$appearance.theme === id} class:on={$appearance.theme === id} on:click={() => set({ theme: id })}>
          <svelte:component this={themeIcons[id]} size={14} />{$t('appearance.theme.' + id)}
        </button>
      {/each}
    </div>
  </div>

  <div class="group">
    <span class="label" id="ap-accent">{$t('settings.appearance.accent')}</span>
    <div class="swatches" role="radiogroup" aria-labelledby="ap-accent">
      {#each Object.entries(ACCENTS) as [id, a]}
        <button
          class="swatch"
          role="radio"
          aria-checked={$appearance.accent === id}
          aria-label={a.label}
          title={a.label}
          style="--swatch: {a.hex}; --swatch-on: {onAccent(a.hex)}"
          on:click={() => set({ accent: id })}
        >
          {#if $appearance.accent === id}<Check size={14} strokeWidth={3} />{/if}
        </button>
      {/each}
      {#each $appearance.customAccents as hex (hex)}
        <span class="custom">
          <button
            class="swatch"
            role="radio"
            aria-checked={$appearance.accent === hex}
            aria-label={$t('settings.appearance.yourColour', { hex })}
            title={$t('settings.appearance.colourDeleteHint', { hex })}
            style="--swatch: {hex}; --swatch-on: {onAccent(hex)}"
            on:click={() => set({ accent: hex })}
            on:keydown={(e) => (e.key === 'Delete' || e.key === 'Backspace') && removeColour(hex)}
          >
            {#if $appearance.accent === hex}<Check size={14} strokeWidth={3} />{/if}
          </button>
          <button class="remove" tabindex="-1" aria-label={$t('settings.appearance.removeColour', { hex })} title={$t('settings.appearance.remove')} on:click={() => removeColour(hex)}>
            <X size={10} strokeWidth={3} />
          </button>
        </span>
      {/each}
      <span class="picker" bind:this={pickerEl}>
        <button
          class="swatch wheel-button"
          aria-label={$t('settings.appearance.pickAnyColour')}
          title={$t('settings.appearance.pickAnyColour')}
          aria-haspopup="dialog"
          aria-expanded={wheelOpen}
          on:click={() => (wheelOpen = !wheelOpen)}
        >
          <Plus size={14} strokeWidth={3} />
        </button>
        {#if wheelOpen}
          <AccentWheel initial={accentHex($appearance.accent)} on:pick={addColour} on:close={() => (wheelOpen = false)} />
        {/if}
      </span>
    </div>
  </div>

  <label class="group">
    <span class="label">{$t('settings.appearance.size')}</span>
    <select value={String($appearance.scale)} on:change={(e) => set({ scale: Number(e.currentTarget.value) })}>
      {#each SCALES as s}
        <option value={String(s)}>{Math.round(s * 100)}%{s === 1 ? ' (default)' : ''}</option>
      {/each}
    </select>
  </label>
</div>

<label class="check motion">
  <input type="checkbox" checked={$appearance.motion} on:change={(e) => set({ motion: e.currentTarget.checked })} />
  {$t('settings.appearance.animations')}
</label>
<p class="hint">
  {$t('settings.appearance.animationsHint')}
  {#if $systemReducesMotion}
    {$t('settings.appearance.reducedMotion')}
  {/if}
</p>

<style>
  .motion {
    margin-top: 18px;
  }
  .options {
    display: flex;
    flex-wrap: wrap;
    gap: 18px 28px;
    /* Labels in one line across the top. The swatches run to two rows, and
       aligned to the bottom the other labels sat a row below theirs. */
    align-items: flex-start;
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
  /* Two rows of nine. */
  .swatches {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    padding: 3px 0;
    max-width: 316px;
  }
  .swatch {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    border: none;
    background: var(--swatch);
    color: var(--swatch-on);
    display: grid;
    place-items: center;
    cursor: pointer;
    box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.15);
    transition: transform 0.16s cubic-bezier(0.2, 0.7, 0.2, 1.4);
  }
  .swatch:hover {
    transform: scale(1.08);
  }
  .swatch:active {
    transform: scale(0.92);
    transition-duration: 0.06s;
  }
  /* The tick pops in on the colour just chosen. */
  .swatch :global(svg) {
    animation: tick-pop 0.24s cubic-bezier(0.2, 0.7, 0.2, 1.5) backwards;
  }
  @keyframes tick-pop {
    from {
      transform: scale(0.3);
      opacity: 0;
    }
  }
  .swatch[aria-checked='true'] {
    box-shadow:
      0 0 0 2px var(--bg-raised),
      0 0 0 4px var(--swatch);
  }
  /* A picked colour: removable from a small cross that shows on hover. */
  .custom,
  .picker {
    position: relative;
    display: inline-flex;
  }
  .remove {
    position: absolute;
    top: -5px;
    right: -5px;
    width: 15px;
    height: 15px;
    padding: 0;
    border-radius: 50%;
    border: 1px solid var(--border-strong);
    background: var(--bg-raised);
    color: var(--text-dim);
    display: grid;
    place-items: center;
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.12s;
  }
  .custom:hover .remove,
  .custom:focus-within .remove {
    opacity: 1;
  }
  .remove:hover {
    color: var(--text);
  }
  /* The wheel's own button: every hue, with a plus over it. */
  .wheel-button {
    background:
      radial-gradient(closest-side, rgba(255, 255, 255, 0.55), rgba(255, 255, 255, 0)),
      conic-gradient(#f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00);
    color: #fff;
  }
  .wheel-button :global(svg) {
    filter: drop-shadow(0 0 2px rgba(0, 0, 0, 0.7));
    animation: none;
  }
  .wheel-button[aria-expanded='true'] {
    box-shadow:
      0 0 0 2px var(--bg-raised),
      0 0 0 4px var(--border-strong);
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
