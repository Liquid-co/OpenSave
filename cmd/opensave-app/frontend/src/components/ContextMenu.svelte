<script>
  // Draws whatever lib/contextmenu.js was asked to open. Kept inside the
  // window: a menu opened near the right or bottom edge flips to the other
  // side of the pointer instead of running off it.
  import { tick } from 'svelte';
  import { contextMenu, closeMenu } from '../lib/contextmenu.js';

  let el;
  let pos = { left: 0, top: 0 };

  $: if ($contextMenu) place($contextMenu);

  // The pointer is reported in window pixels, but with the app drawn larger
  // or smaller (Settings → Appearance → Size) a fixed position is scaled by
  // that size — so the pointer is brought into the page's own units first.
  //
  // Measured at the top left, out of sight, before being placed: measured
  // where the pointer is, a menu near the right edge is squeezed to fit the
  // space left there, and the flip is worked out from that squeezed size.
  async function place(menu) {
    const zoom = Number(document.documentElement.style.zoom) || 1;
    const x = menu.x / zoom;
    const y = menu.y / zoom;
    pos = { left: 0, top: 0, measuring: true };
    await tick();
    if (!el) return;
    // offsetWidth, not the bounding box: the opening animation scales it.
    const width = el.offsetWidth;
    const height = el.offsetHeight;
    const vw = window.innerWidth / zoom;
    const vh = window.innerHeight / zoom;
    pos = {
      left: x + width > vw - 8 ? Math.max(8, x - width) : x,
      top: y + height > vh - 8 ? Math.max(8, y - height) : y
    };
    await tick();
    focusable()[0]?.focus();
  }

  const focusable = () => [...(el?.querySelectorAll('button:not(:disabled)') ?? [])];

  function choose(item) {
    closeMenu();
    item.run();
  }

  function onKeydown(e) {
    if (!$contextMenu) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      closeMenu();
      return;
    }
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      const items = focusable();
      const i = items.indexOf(document.activeElement);
      const next = e.key === 'ArrowDown' ? (i + 1) % items.length : (i - 1 + items.length) % items.length;
      items[next]?.focus();
    }
  }
</script>

<svelte:window
  on:keydown={onKeydown}
  on:blur={closeMenu}
  on:resize={closeMenu}
/>

{#if $contextMenu}
  <!-- Anywhere outside the menu closes it, the way a system menu does. -->
  <div class="catcher" role="presentation" on:mousedown={closeMenu} on:contextmenu|preventDefault={closeMenu} on:wheel={closeMenu}></div>
  <div class="menu" class:measuring={pos.measuring} role="menu" bind:this={el} style="left: {pos.left}px; top: {pos.top}px">
    {#each $contextMenu.items as item}
      {#if item === null}
        <div class="sep" role="separator"></div>
      {:else}
        <button role="menuitem" class:danger={item.danger} disabled={item.disabled} on:click={() => choose(item)}>
          <span class="icon">{#if item.icon}<svelte:component this={item.icon} size={15} />{/if}</span>
          <span class="label">{item.label}</span>
          {#if item.hint}<span class="hint-text">{item.hint}</span>{/if}
        </button>
      {/if}
    {/each}
  </div>
{/if}

<style>
  .catcher {
    position: fixed;
    inset: 0;
    z-index: 190;
  }
  .menu {
    position: fixed;
    z-index: 200;
    min-width: 220px;
    padding: 5px;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: 10px;
    box-shadow: var(--shadow);
    animation: pop 0.1s ease-out;
  }
  .menu.measuring {
    visibility: hidden;
    animation: none;
  }
  @keyframes pop {
    from {
      opacity: 0;
      transform: scale(0.97);
    }
  }
  button {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    padding: 7px 10px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.86rem;
    text-align: left;
    white-space: nowrap;
    cursor: pointer;
  }
  button:hover:not(:disabled),
  button:focus-visible {
    background: var(--bg-hover);
    outline: none;
  }
  button:disabled {
    color: var(--text-faint);
    cursor: default;
  }
  .icon {
    display: inline-flex;
    width: 15px;
    color: var(--text-dim);
  }
  .label {
    flex: 1;
  }
  .hint-text {
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  button.danger {
    color: var(--danger-text);
  }
  button.danger .icon {
    color: var(--danger-text);
  }
  .sep {
    height: 1px;
    margin: 5px 6px;
    background: var(--border);
  }
</style>
