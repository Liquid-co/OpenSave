<script>
  // A button with a second half that opens a menu: the left does the usual
  // thing, the arrow offers the other ways to do it. The menu drops from the
  // button itself, right-aligned under it and at least as wide (Dropdown).
  import { createEventDispatcher } from 'svelte';
  import ChevronDown from 'lucide-svelte/icons/chevron-down';
  import Dropdown from './Dropdown.svelte';

  export let label = '';
  /** A lucide icon component, or null. */
  export let icon = null;
  export let primary = false;
  export let disabled = false;
  export let title = '';
  /** What the arrow offers: [{label, hint?, icon?, run, disabled?} | null]. */
  export let items = [];
  export let menuLabel = '';

  const dispatch = createEventDispatcher();
  let open = false;
  let group;
  let arrow;
  let width = 0;

  // A press anywhere else closes it; a press on the arrow is the arrow's own
  // toggle, not "outside", so the menu does not close and reopen.
  function outside(e) {
    if (open && !group?.contains(e.target)) open = false;
  }
</script>

<svelte:window on:mousedown={outside} />

<div class="split" class:primary class:open role="group" bind:this={group} bind:clientWidth={width}>
  <button class="btn main" class:primary {disabled} {title} on:click={() => dispatch('click')}>
    {#if icon}<svelte:component this={icon} size={17} />{/if}{label}
  </button>
  <button
    class="btn arrow"
    class:primary
    bind:this={arrow}
    disabled={items.every((i) => !i || i.disabled)}
    aria-label={menuLabel}
    title={menuLabel}
    aria-haspopup="menu"
    aria-expanded={open}
    on:click={() => (open = !open)}
  >
    <span class="chev"><ChevronDown size={16} /></span>
  </button>
  <Dropdown {items} bind:open minWidth={width} label={menuLabel} on:close={(e) => e.detail.giveBack && arrow?.focus()} />
</div>

<style>
  .split {
    position: relative;
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
    width: 36px;
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
  .open .arrow:not(.primary) {
    background: var(--btn-bg-hover);
  }
  .open .arrow.primary {
    background: var(--accent-fill-hover);
  }
  .chev {
    display: inline-flex;
    transition: transform 0.18s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .open .chev {
    transform: rotate(180deg);
  }
</style>
