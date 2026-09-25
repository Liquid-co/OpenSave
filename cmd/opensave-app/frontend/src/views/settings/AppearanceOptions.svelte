<script>
  // Theme, accent colour, size and motion, for this device. Applied as they
  // are chosen; see lib/appearance.js.
  import Check from 'lucide-svelte/icons/check';
  import Moon from 'lucide-svelte/icons/moon';
  import Sun from 'lucide-svelte/icons/sun';
  import MonitorCog from 'lucide-svelte/icons/monitor-cog';
  import { appearance, THEMES, ACCENTS, SCALES } from '../../lib/appearance.js';
  import { systemReducesMotion } from '../../lib/motion.js';
  import { CONTROLLER_MODES, controllerOn, padUsed } from '../../lib/controller.js';
  import { settings } from '../../lib/stores.js';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';

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

<div class="group controller">
  <span class="label" id="ap-controller">Controller</span>
  <div class="segmented" role="radiogroup" aria-labelledby="ap-controller">
    {#each Object.entries(CONTROLLER_MODES) as [id, label]}
      <button role="radio" aria-checked={$appearance.controller === id} class:on={$appearance.controller === id} on:click={() => set({ controller: id })}>
        {#if id === 'on'}<Gamepad2 size={14} />{/if}{label}
      </button>
    {/each}
  </div>
  <p class="hint">
    Move around with a gamepad or the arrow keys: A presses, B backs out, X opens a game's menu, Y or Start opens
    Ctrl+K, the shoulder buttons change page.
    {#if $appearance.controller === 'auto'}
      {controllerOn('auto', { deviceType: $settings?.deviceType, used: $padUsed })
        ? 'On now — a controller was used, or this device is a Steam Deck or handheld.'
        : 'Turns on when a controller is used, and on a Steam Deck or handheld.'}
    {/if}
  </p>
</div>

<label class="check motion">
  <input type="checkbox" checked={$appearance.motion} on:change={(e) => set({ motion: e.currentTarget.checked })} />
  Animations
</label>
<p class="hint">
  Small movements as pages, dialogs and your library come into view, and as you point at a game.
  {#if $systemReducesMotion}
    Off for now whatever this says: Windows is asking apps to reduce motion.
  {/if}
</p>

<style>
  .motion {
    margin-top: 18px;
  }
  .controller {
    margin-top: 18px;
  }
  .controller .hint {
    margin: 2px 0 0;
  }
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
