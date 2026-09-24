<script>
  // The large dialog the scan, the cloud browser and the backup import/export
  // open in: a dimmed backdrop, a panel with a title row, and whatever the
  // screen puts underneath. Each of those built its own copy, and the copies
  // had started to drift.
  //
  // It closes from the ✕, from Escape while focus is inside it, and from a
  // click that starts and ends on the backdrop (see backdrop.js for why both).
  // `closable` turns all three off at once, for a dialog that must not be
  // dismissed while it is writing something.
  import { backdropClose } from '../../lib/backdrop.js';

  export let title = '';
  export let onClose = () => {};
  export let closable = true;
  /** Widest the panel gets, in pixels; it narrows with the window. */
  export let width = 760;
  export let height = 'min(78vh, 720px)';
  export let maxHeight = 'none';

  const close = () => {
    if (closable) onClose();
  };
</script>

<div
  class="overlay"
  use:backdropClose={close}
  on:keydown={(e) => e.key === 'Escape' && close()}
  role="presentation"
>
  <div class="panel" style="width: min({width}px, 100%); height: {height}; max-height: {maxHeight}">
    <div class="head">
      <div>
        <h2>{title}</h2>
        {#if $$slots.sub}<p class="sub"><slot name="sub" /></p>{/if}
      </div>
      <div class="head-actions">
        <slot name="actions" />
        <button class="btn icon" disabled={!closable} on:click={close} title="Close">✕</button>
      </div>
    </div>
    <slot />
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.62);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 80;
    padding: 32px;
  }
  .panel {
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    display: flex;
    flex-direction: column;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
  }
  .head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    padding: 20px 22px 14px;
    border-bottom: 1px solid var(--border);
  }
  .head h2 {
    font-size: 1.2rem;
  }
  .sub {
    font-size: 0.84rem;
    color: var(--text-faint);
    margin-top: 3px;
  }
  .head-actions {
    display: flex;
    gap: 8px;
    align-items: center;
  }
</style>
