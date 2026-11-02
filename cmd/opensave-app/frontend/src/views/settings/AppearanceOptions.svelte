<script>
  // Theme, accent colour, size and motion, for this device. Applied as they
  // are chosen; see lib/appearance.js.
  import Check from 'lucide-svelte/icons/check';
  import Moon from 'lucide-svelte/icons/moon';
  import Sun from 'lucide-svelte/icons/sun';
  import MonitorCog from 'lucide-svelte/icons/monitor-cog';
  import { appearance, THEMES, ACCENTS, SCALES, onAccent } from '../../lib/appearance.js';
  import { systemReducesMotion, slidingIndicator } from '../../lib/motion.js';
  import { locale, LOCALES, t, SYSTEM_LOCALE } from '../../lib/i18n.js';

  const set = (patch) => appearance.update((v) => ({ ...v, ...patch }));
  const themeIcons = { dark: Moon, light: Sun, system: MonitorCog };
</script>

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
    <span class="label" id="ap-theme">{$t('appearanceOptions.theme')}</span>
    <div class="segmented" role="radiogroup" aria-labelledby="ap-theme" use:slidingIndicator={{ mode: 'fill' }}>
      {#each Object.entries(THEMES) as [id, label]}
        <button role="radio" aria-checked={$appearance.theme === id} class:on={$appearance.theme === id} on:click={() => set({ theme: id })}>
          <svelte:component this={themeIcons[id]} size={14} />{$t('appearance.theme.' + id)}
        </button>
      {/each}
    </div>
  </div>

  <div class="group">
    <span class="label" id="ap-accent">{$t('appearanceOptions.accentColour')}</span>
    <div class="swatches" role="radiogroup" aria-labelledby="ap-accent">
      {#each Object.entries(ACCENTS) as [id, a]}
        <button
          class="swatch"
          role="radio"
          aria-checked={$appearance.accent === id}
          aria-label={$t('appearance.accent.' + id)}
          title={$t('appearance.accent.' + id)}
          style="--swatch: {a.hex}; --swatch-on: {onAccent(a.hex)}"
          on:click={() => set({ accent: id })}
        >
          {#if $appearance.accent === id}<Check size={14} strokeWidth={3} />{/if}
        </button>
      {/each}
    </div>
  </div>

  <label class="group">
    <span class="label">{$t('cloudTab.size')}</span>
    <select value={String($appearance.scale)} on:change={(e) => set({ scale: Number(e.currentTarget.value) })}>
      {#each SCALES as s}
        <option value={String(s)}>{Math.round(s * 100)}%{s === 1 ? $t('appearanceOptions.default') : ''}</option>
      {/each}
    </select>
  </label>
</div>

<label class="check motion">
  <input type="checkbox" checked={$appearance.motion} on:change={(e) => set({ motion: e.currentTarget.checked })} />
  {$t('appearanceOptions.animations')}
</label>
<p class="hint">
  {$t('appearanceOptions.smallMovementsAsPagesDialogsAndYour')}
  {#if $systemReducesMotion}
    {$t('appearanceOptions.offForNowWhateverThisSaysWindows')}
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
