<script>
  // Appearance, for this device: theme, accent, the game page's own colours,
  // the Library's background, the sidebar, size, motion and the Library's
  // look. Applied as they are chosen (lib/appearance.js); none of it waits on
  // the daemon.
  import SettingGroup from '../../../components/ui/SettingGroup.svelte';
  import SettingRow from '../../../components/ui/SettingRow.svelte';
  import Switch from '../../../components/ui/Switch.svelte';
  import Segmented from '../../../components/ui/Segmented.svelte';
  import LibraryViewOptions from '../../home/LibraryViewOptions.svelte';
  import AccentWheel from '../AccentWheel.svelte';
  import Check from 'lucide-svelte/icons/check';
  import Moon from 'lucide-svelte/icons/moon';
  import Sun from 'lucide-svelte/icons/sun';
  import MonitorCog from 'lucide-svelte/icons/monitor-cog';
  import Plus from 'lucide-svelte/icons/plus';
  import X from 'lucide-svelte/icons/x';
  import { appearance, THEMES, ACCENTS, SCALES, BACKDROPS, onAccent, accentHex, withCustomAccent, withoutCustomAccent } from '../../../lib/appearance.js';
  import { systemReducesMotion } from '../../../lib/motion.js';
  import { sidebar, chooseSidebar, SIDEBAR_MODES } from '../../../lib/shell.js';
  import { t } from '../../../lib/i18n.js';

  const set = (patch) => appearance.update((v) => ({ ...v, ...patch }));
  const themeIcons = { dark: Moon, light: Sun, system: MonitorCog };

  $: themeOptions = Object.keys(THEMES).map((id) => ({ id, label: $t('appearance.theme.' + id), icon: themeIcons[id] }));
  $: backdropOptions = Object.keys(BACKDROPS).map((id) => ({ id, label: $t('appearance.backdrop.' + id) }));
  $: sidebarOptions = SIDEBAR_MODES.map((id) => ({ id, label: $t('shell.sidebar.' + id) }));

  // The colour wheel, open beside its button until a colour is added or it
  // is dismissed: Escape, or a click anywhere else.
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

<svelte:window on:pointerdown={outside} on:keydown={(e) => wheelOpen && e.key === 'Escape' && (e.stopPropagation(), (wheelOpen = false))} />

<SettingGroup title={$t('settings.appearance.theme')}>
  <SettingRow id="theme" label={$t('settings.appearance.theme')}>
    <Segmented options={themeOptions} value={$appearance.theme} label={$t('settings.appearance.theme')} on:change={(e) => set({ theme: e.detail })} />
  </SettingRow>
  <SettingRow id="accent" label={$t('settings.appearance.accent')} under>
    <div class="swatches" role="radiogroup" aria-label={$t('settings.appearance.accent')}>
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
  </SettingRow>
  <SettingRow id="coverColours" label={$t('settings.appearance.coverColours')} hint={$t('settings.appearance.coverColoursHint')}>
    <Switch checked={$appearance.coverColours} label={$t('settings.appearance.coverColours')} on:change={(e) => set({ coverColours: e.detail })} />
  </SettingRow>
</SettingGroup>

<SettingGroup title={$t('settings.appearance.layout')}>
  <SettingRow id="backdrop" label={$t('settings.appearance.backdrop')}>
    <Segmented options={backdropOptions} value={$appearance.backdrop} label={$t('settings.appearance.backdrop')} on:change={(e) => set({ backdrop: e.detail })} />
  </SettingRow>
  <SettingRow id="sidebar" label={$t('settings.appearance.sidebar')}>
    <Segmented options={sidebarOptions} value={$sidebar.mode} label={$t('settings.appearance.sidebar')} on:change={(e) => sidebar.update((v) => chooseSidebar(v, e.detail))} />
  </SettingRow>
  <SettingRow id="size" label={$t('settings.appearance.size')} control="s-scale">
    <select id="s-scale" value={String($appearance.scale)} on:change={(e) => set({ scale: Number(e.currentTarget.value) })}>
      {#each SCALES as s}
        <option value={String(s)}>{Math.round(s * 100)}%{s === 1 ? ` ${$t('settings.appearance.scaleDefault')}` : ''}</option>
      {/each}
    </select>
  </SettingRow>
  <SettingRow id="animations" label={$t('settings.appearance.animations')}>
    <svelte:fragment slot="hint">
      {$t('settings.appearance.animationsHint')}
      {#if $systemReducesMotion}{$t('settings.appearance.reducedMotion')}{/if}
    </svelte:fragment>
    <Switch checked={$appearance.motion} label={$t('settings.appearance.animations')} on:change={(e) => set({ motion: e.detail })} />
  </SettingRow>
</SettingGroup>

<SettingGroup title={$t('settings.general.library')}>
  <SettingRow id="library" label={$t('settings.general.library')} hint={$t('settings.general.libraryHint')} under>
    <LibraryViewOptions />
  </SettingRow>
</SettingGroup>

<style>
  .swatches {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    padding: 3px 0;
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
  .swatch :global(svg) {
    animation: tick-pop 0.24s cubic-bezier(0.2, 0.7, 0.2, 1.5) backwards;
  }
  @keyframes tick-pop {
    from {
      transform: scale(0.3);
    }
  }
  .swatch[aria-checked='true'] {
    box-shadow:
      0 0 0 2px var(--bg-raised),
      0 0 0 4px var(--swatch);
  }
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
</style>
