<script>
  // A menu that drops from the control it belongs to: under it, lined up with
  // its edge, at least as wide as it, and folding back into it. For the
  // Library's split buttons and its Sort. (Right-click menus are
  // ContextMenu: they open where the pointer is, which is not this.)
  //
  // Keys: the arrows move between items, Home and End to either end, Enter
  // or Space chooses, Escape closes and gives focus back to the control.
  import { createEventDispatcher, tick } from 'svelte';
  import Check from 'lucide-svelte/icons/check';
  import { unfold } from '../../lib/motion.js';

  /** [{label, hint?, icon?, checked?, disabled?, danger?, run} | null (a rule)] */
  export let items = [];
  export let open = false;
  /** Which edge of the control the menu lines up with. */
  export let align = 'right';
  /** The control's width, so the menu is never narrower than it. */
  export let minWidth = 0;
  export let label = '';

  const dispatch = createEventDispatcher();
  let menu;

  $: if (open) focusFirst();
  async function focusFirst() {
    await tick();
    buttons()[0]?.focus({ preventScroll: true });
  }
  const buttons = () => [...(menu?.querySelectorAll('button:not(:disabled)') ?? [])];

  function close(giveBack = true) {
    open = false;
    dispatch('close', { giveBack });
  }
  function choose(item) {
    if (item.disabled) return;
    close(false);
    item.run?.();
  }
  function keys(e) {
    const all = buttons();
    const i = all.indexOf(document.activeElement);
    const go = (j) => {
      e.preventDefault();
      all[(j + all.length) % all.length]?.focus();
    };
    if (e.key === 'ArrowDown') go(i + 1);
    else if (e.key === 'ArrowUp') go(i - 1);
    else if (e.key === 'Home') go(0);
    else if (e.key === 'End') go(all.length - 1);
    else if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      close(true);
    } else if (e.key === 'Tab') close(false);
  }
</script>

{#if open}
  <div
    class="dropdown align-{align}"
    role="menu"
    aria-label={label}
    tabindex="-1"
    bind:this={menu}
    style="min-width: {Math.max(minWidth, 220)}px"
    on:keydown={keys}
    transition:unfold={{ origin: `top ${align}`, duration: 150 }}
  >
    {#each items as item, i (i)}
      {#if item === null}
        <div class="rule" role="separator"></div>
      {:else}
        <button role="menuitem" class:danger={item.danger} class:two={!!item.hint} disabled={item.disabled} on:click={() => choose(item)}>
          {#if item.icon}
            <span class="icon"><svelte:component this={item.icon} size={16} /></span>
          {:else if item.checked !== undefined}
            <span class="tick">{#if item.checked}<Check size={15} />{/if}</span>
          {/if}
          <span class="text">
            <span class="label">{item.label}</span>
            {#if item.hint}<span class="hint">{item.hint}</span>{/if}
          </span>
          {#if item.icon && item.checked}<span class="tick end"><Check size={15} /></span>{/if}
        </button>
      {/if}
    {/each}
  </div>
{/if}

<style>
  .dropdown {
    position: absolute;
    top: calc(100% + 6px);
    z-index: 70;
    padding: 6px;
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    box-shadow: var(--shadow);
    display: flex;
    flex-direction: column;
    gap: 2px;
    outline: none;
  }
  .align-right {
    right: 0;
  }
  .align-left {
    left: 0;
  }
  button {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    min-height: 38px;
    padding: 6px 10px;
    border: none;
    border-radius: var(--radius);
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-base);
    text-align: left;
    cursor: pointer;
    white-space: nowrap;
  }
  button.two {
    min-height: 50px;
  }
  button:hover:not(:disabled),
  button:focus-visible {
    background: var(--bg-hover);
    outline: none;
  }
  button:disabled {
    opacity: 0.45;
    cursor: default;
  }
  button.danger {
    color: var(--danger-text);
  }
  .icon {
    width: 32px;
    height: 32px;
    flex-shrink: 0;
    border-radius: calc(9px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--accent-text);
  }
  .tick {
    width: 16px;
    flex-shrink: 0;
    display: inline-flex;
    color: var(--accent-text);
  }
  .tick.end {
    margin-left: auto;
  }
  .text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .label {
    font-weight: 500;
  }
  .two .label {
    font-weight: 600;
  }
  .hint {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .rule {
    height: 1px;
    margin: 4px 6px;
    background: var(--border);
  }
</style>
