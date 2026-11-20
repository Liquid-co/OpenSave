<script>
  // An on/off switch. Fires `change` with the new value; the caller decides
  // whether to keep it (bind `checked`, or set it back when a save fails).
  import { createEventDispatcher } from 'svelte';

  export let checked = false;
  export let disabled = false;
  /** Read out in place of a visible label, when there is none beside it. */
  export let label = '';
  const dispatch = createEventDispatcher();

  function toggle() {
    if (disabled) return;
    checked = !checked;
    dispatch('change', checked);
  }
</script>

<button class="sw" class:on={checked} role="switch" aria-checked={checked} aria-label={label || undefined} {disabled} on:click={toggle}>
  <span class="knob"></span>
</button>

<style>
  .sw {
    position: relative;
    flex: none;
    width: 36px;
    height: 21px;
    padding: 0;
    border-radius: 999px;
    border: 1px solid var(--border-strong);
    background: var(--bg-active);
    cursor: pointer;
    transition:
      background 0.18s,
      border-color 0.18s;
  }
  .sw.on {
    background: var(--accent-fill);
    border-color: var(--accent-fill);
  }
  .sw:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .knob {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 15px;
    height: 15px;
    border-radius: 50%;
    background: var(--text-dim);
    transition:
      transform 0.18s cubic-bezier(0.3, 0.7, 0.3, 1),
      background 0.18s;
  }
  .on .knob {
    transform: translateX(15px);
    background: var(--on-accent);
  }
  .sw:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
</style>
