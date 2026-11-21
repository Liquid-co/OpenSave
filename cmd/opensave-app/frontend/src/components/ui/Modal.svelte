<script>
  // The large dialog the scan, the cloud browser and the backup import/export
  // open in: a dimmed backdrop, a panel with a title row, and whatever the
  // screen puts underneath. Each of those built its own copy, and the copies
  // had started to drift.
  //
  // It closes from the close button, from Escape while focus is inside it, and from a
  // click that starts and ends on the backdrop (see backdrop.js for why both).
  // `closable` turns all three off at once, for a dialog that must not be
  // dismissed while it is writing something.
  import { dialogOut } from '../../lib/motion.js';
  import { onMount } from 'svelte';
  import X from 'lucide-svelte/icons/x';
  import { backdropClose } from '../../lib/backdrop.js';
  import { t } from '../../lib/i18n.js';

  export let title = '';
  /** A lucide icon component drawn before the title. */
  export let icon = null;
  export let onClose = () => {};
  export let closable = true;
  /** Widest the panel gets, in pixels; it narrows with the window. */
  export let width = 760;
  export let height = 'min(78vh, 720px)';
  /** Tallest it gets. Never past the window: by default it stops at the
   *  backdrop's padding, and its .body scrolls (see the style below). */
  export let maxHeight = '100%';

  const close = () => {
    if (closable) onClose();
  };

  // Focus comes into the dialog when it opens — unless something in it has
  // already taken it — so Escape closes it straight away and a screen reader
  // is told where it is. Opened from the keyboard, focus was left behind on
  // the page, and Escape went nowhere.
  let panel;
  onMount(() => {
    if (!panel.contains(document.activeElement)) panel.focus();
  });
</script>

<div
  class="overlay"
  out:dialogOut|global
  use:backdropClose={close}
  on:keydown={(e) => e.key === 'Escape' && close()}
  role="presentation"
>
  <div class="panel" bind:this={panel} tabindex="-1" role="dialog" aria-modal="true" aria-label={title} style="width: min({width}px, 100%); height: {height}; max-height: {maxHeight}">
    <div class="head">
      {#if icon}<span class="mark"><svelte:component this={icon} size={20} /></span>{/if}
      <div class="titles">
        <h2>{title}</h2>
        {#if $$slots.sub}<p class="sub"><slot name="sub" /></p>{/if}
      </div>
      <div class="head-actions">
        <slot name="actions" />
        <button class="shut" disabled={!closable} on:click={close} title={$t('common.close')} aria-label={$t('common.close')}><X size={18} /></button>
      </div>
    </div>
    <slot />
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 80;
    padding: 32px;
  }
  .panel {
    --modal-r: 18px;
    outline: none;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--modal-r);
    display: flex;
    flex-direction: column;
    box-shadow: var(--shadow);
  }
  /* A dialog's .body takes the height left between the title and the
     footer, and scrolls when what it holds is taller. Without this a dialog
     sized to its content ran off a short window (a Steam Deck, a laptop),
     and one with a fixed height squeezed its list and cut it off. */
  .panel > :global(.body) {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
  }
  .head {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 20px 20px 16px 22px;
    border-bottom: 1px solid var(--border);
  }
  /* The dialog's subject, in a tinted tile, as on the sync panel. */
  .mark {
    width: 40px;
    height: 40px;
    flex-shrink: 0;
    border-radius: 12px;
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .titles {
    flex: 1;
    min-width: 0;
  }
  .head h2 {
    font-size: var(--text-xl);
    font-weight: 600;
    letter-spacing: -0.005em;
    line-height: 1.25;
  }
  .sub {
    font-size: var(--text-md);
    color: var(--text-faint);
    margin-top: 2px;
  }
  .head-actions {
    display: flex;
    gap: 8px;
    align-items: center;
    align-self: flex-start;
  }
  .shut {
    width: 34px;
    height: 34px;
    display: grid;
    place-items: center;
    border: none;
    border-radius: 10px;
    background: transparent;
    color: var(--text-dim);
    cursor: pointer;
    transition:
      background 0.12s,
      color 0.12s,
      transform 0.16s cubic-bezier(0.2, 0.7, 0.2, 1.4);
  }
  .shut:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text);
  }
  .shut:hover:not(:disabled) :global(svg) {
    transform: rotate(90deg);
  }
  .shut :global(svg) {
    transition: transform 0.2s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .shut:disabled {
    opacity: 0.4;
    cursor: default;
  }
</style>
