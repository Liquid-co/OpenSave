<script>
  // A colour wheel for picking any accent: hue around the circle, saturation
  // out from the centre, brightness on the slider below. Shown in a popover by
  // the Appearance section; says what was picked through on:pick, or on:close.
  import { createEventDispatcher, onMount } from 'svelte';
  import { hsvToHex, hexToHsv, normalizeHex, onAccent } from '../../lib/appearance.js';
  import { t } from '../../lib/i18n.js';

  export let initial = '#8a63f4';
  /** Which edge of its button the wheel lines up with. */
  export let align = 'right';
  /** What choosing says: adding a colour to the accents, or using one. */
  export let confirm = '';

  const dispatch = createEventDispatcher();
  let { h, s, v } = hexToHsv(initial);
  let wheel;
  let hexText = hsvToHex(h, s, v);
  let dragging = false;

  $: hex = hsvToHex(h, s, v);
  $: if (!hexFocused) hexText = hex;
  // The marker sits where the colour is: angle from the top, clockwise, as
  // the conic gradient draws hue; distance from the centre for saturation.
  $: markerX = 50 + 50 * s * Math.sin((h * Math.PI) / 180);
  $: markerY = 50 - 50 * s * Math.cos((h * Math.PI) / 180);
  let hexFocused = false;

  function pickAt(e) {
    const r = wheel.getBoundingClientRect();
    const dx = e.clientX - (r.left + r.width / 2);
    const dy = e.clientY - (r.top + r.height / 2);
    h = ((Math.atan2(dx, -dy) * 180) / Math.PI + 360) % 360;
    s = Math.min(1, Math.hypot(dx, dy) / (r.width / 2));
  }

  function down(e) {
    dragging = true;
    wheel.setPointerCapture?.(e.pointerId);
    pickAt(e);
  }
  const move = (e) => dragging && pickAt(e);
  const up = () => (dragging = false);

  // Arrows move around the wheel (left/right) and in and out (up/down).
  function wheelKey(e) {
    const step = e.shiftKey ? 15 : 3;
    if (e.key === 'ArrowLeft') h = (h - step + 360) % 360;
    else if (e.key === 'ArrowRight') h = (h + step) % 360;
    else if (e.key === 'ArrowUp') s = Math.min(1, s + step / 100);
    else if (e.key === 'ArrowDown') s = Math.max(0, s - step / 100);
    else return;
    e.preventDefault();
  }

  function typed() {
    const n = normalizeHex(hexText);
    if (n) ({ h, s, v } = hexToHsv(n));
  }

  onMount(() => wheel?.focus());
</script>

<div class="wheel-pop align-{align}" role="dialog" aria-label={$t('colourWheel.aria')}>
  <div
    class="wheel"
    bind:this={wheel}
    role="slider"
    tabindex="0"
    aria-label="Hue and saturation"
    aria-valuetext={hex}
    aria-valuenow={Math.round(h)}
    aria-valuemin="0"
    aria-valuemax="360"
    style="--dim: {1 - v}"
    on:pointerdown={down}
    on:pointermove={move}
    on:pointerup={up}
    on:pointercancel={up}
    on:keydown={wheelKey}
  >
    <span class="marker" style="left: {markerX}%; top: {markerY}%; background: {hex}"></span>
  </div>

  <label class="value">
    <span>Brightness</span>
    <input
      type="range"
      min="0"
      max="1"
      step="0.01"
      bind:value={v}
      style="--full: {hsvToHex(h, s, 1)}"
    />
  </label>

  <div class="row">
    <span class="preview" style="background: {hex}; color: {onAccent(hex)}">Aa</span>
    <input
      class="hex"
      aria-label="Colour as hex"
      spellcheck="false"
      maxlength="7"
      bind:value={hexText}
      on:input={typed}
      on:focus={() => (hexFocused = true)}
      on:blur={() => ((hexFocused = false), (hexText = hex))}
      on:keydown={(e) => e.key === 'Enter' && dispatch('pick', hex)}
    />
  </div>

  <div class="actions">
    <button class="btn small ghost" on:click={() => dispatch('close')}>{$t('common.cancel')}</button>
    <button class="btn small primary" on:click={() => dispatch('pick', hex)}>{confirm || $t('colourWheel.add')}</button>
  </div>
</div>

<style>
  .wheel-pop {
    position: absolute;
    z-index: 30;
    top: calc(100% + 8px);
    right: 0;
    width: 220px;
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg, calc(12px * var(--round, 1)));
    box-shadow: 0 16px 44px rgba(0, 0, 0, 0.5);
  }
  .wheel {
    position: relative;
    width: 192px;
    height: 192px;
    border-radius: 50%;
    cursor: crosshair;
    touch-action: none;
    /* White at the centre fading out to the full hue at the rim, darkened
       as a whole by the brightness below. */
    background:
      radial-gradient(closest-side, #fff, rgba(255, 255, 255, 0)),
      conic-gradient(#f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00);
  }
  .wheel::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: 50%;
    background: #000;
    opacity: var(--dim);
    pointer-events: none;
  }
  .wheel:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 3px;
  }
  .marker {
    position: absolute;
    z-index: 1;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    transform: translate(-50%, -50%);
    border: 2px solid #fff;
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.45), 0 2px 6px rgba(0, 0, 0, 0.4);
    pointer-events: none;
  }
  .value {
    display: flex;
    flex-direction: column;
    gap: 5px;
    font-size: 0.78rem;
    color: var(--text-dim);
  }
  .value input {
    width: 100%;
    accent-color: var(--full);
    height: 10px;
    border-radius: calc(5px * var(--round, 1));
    appearance: none;
    background: linear-gradient(to right, #000, var(--full));
  }
  .value input::-webkit-slider-thumb {
    appearance: none;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.45);
    cursor: pointer;
  }
  .row {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .preview {
    width: 34px;
    height: 34px;
    border-radius: calc(8px * var(--round, 1));
    display: grid;
    place-items: center;
    font-size: 0.8rem;
    font-weight: 700;
    box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.15);
    flex-shrink: 0;
  }
  .hex {
    flex: 1;
    min-width: 0;
    padding: 7px 9px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: calc(8px * var(--round, 1));
    color: var(--text);
    font-family: ui-monospace, monospace;
    font-size: 0.86rem;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
  .wheel-pop.align-left {
    left: 0;
    right: auto;
  }
</style>
