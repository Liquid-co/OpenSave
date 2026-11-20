<script>
  // A button with a second half that opens a menu: the left does the usual
  // thing, the arrow offers the other ways to do it (lib/contextmenu.js
  // draws the menu, so it looks and moves like every other menu).
  import { createEventDispatcher } from 'svelte';
  import ChevronDown from 'lucide-svelte/icons/chevron-down';
  import { openMenu } from '../../lib/contextmenu.js';

  export let label = '';
  /** A lucide icon component, or null. */
  export let icon = null;
  export let primary = false;
  export let disabled = false;
  export let title = '';
  /** What the arrow offers: [{label, icon?, run, disabled?}]. */
  export let items = [];
  export let menuLabel = '';

  const dispatch = createEventDispatcher();

  function more(e) {
    // Below the button, wherever it was clicked.
    const r = e.currentTarget.parentElement.getBoundingClientRect();
    e.stopPropagation();
    openMenu({ preventDefault() {}, stopPropagation() {}, clientX: r.left, clientY: r.bottom + 6, currentTarget: e.currentTarget }, items);
  }
</script>

<div class="split" class:primary role="group">
  <button class="btn main" class:primary {disabled} {title} on:click={() => dispatch('click')}>
    {#if icon}<svelte:component this={icon} size={17} />{/if}{label}
  </button>
  <button class="btn arrow" class:primary disabled={disabled && items.every((i) => i.disabled)} aria-label={menuLabel} title={menuLabel} aria-haspopup="menu" on:click={more}>
    <ChevronDown size={15} />
  </button>
</div>

<style>
  .split {
    display: inline-flex;
  }
  .btn {
    height: 40px;
  }
  .main {
    padding: 0 16px;
    font-size: var(--text-base);
    border-top-right-radius: 0;
    border-bottom-right-radius: 0;
  }
  .arrow {
    width: 34px;
    padding: 0;
    border-top-left-radius: 0;
    border-bottom-left-radius: 0;
    border-left-color: transparent;
    margin-left: 1px;
  }
  .primary .arrow {
    margin-left: 0;
    border-left: 1px solid rgba(255, 255, 255, 0.22);
  }
</style>
