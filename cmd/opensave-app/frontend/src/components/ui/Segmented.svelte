<script>
  // A few choices side by side, one picked, a marker sliding to it. Fires
  // `change` with the id chosen.
  import { createEventDispatcher } from 'svelte';
  import { slidingIndicator } from '../../lib/motion.js';

  /** [{id, label, icon?}] */
  export let options = [];
  export let value;
  export let label = '';
  export let disabled = false;
  const dispatch = createEventDispatcher();
</script>

<div class="seg" role="radiogroup" aria-label={label || undefined} use:slidingIndicator={{ mode: 'fill', className: 'seg-mark' }}>
  {#each options as o (o.id)}
    <button role="radio" aria-checked={value === o.id} class:on={value === o.id} {disabled} on:click={() => dispatch('change', o.id)}>
      {#if o.icon}<svelte:component this={o.icon} size={14} />{/if}{o.label}
    </button>
  {/each}
</div>

<style>
  .seg {
    position: relative;
    display: inline-flex;
    flex-wrap: wrap;
    gap: 2px;
    padding: 3px;
    border-radius: 10px;
    background: var(--bg);
    border: 1px solid var(--border);
  }
  button {
    position: relative;
    z-index: 1;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 28px;
    padding: 0 11px;
    border: 0;
    border-radius: 7px;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-md);
    font-weight: 500;
    white-space: nowrap;
    cursor: pointer;
    transition: color 0.15s;
  }
  button:hover:not(:disabled) {
    color: var(--text);
  }
  button.on {
    color: var(--text);
    font-weight: 600;
  }
  /* The marker (lib/motion.js slidingIndicator) behind the one picked. */
  .seg :global(.seg-mark) {
    border-radius: 7px;
    background: var(--bg-active);
    box-shadow: inset 0 0 0 1px var(--border-strong);
  }
  /* Before the marker has measured: the picked one carries its own fill. */
  .seg:not(:global(.slides)) button.on {
    background: var(--bg-active);
  }
</style>
