<script>
  import { toasts, dismissToast } from '../lib/stores.js';
  // `t` is a toast in the loop below, so the store comes in under its own name.
  import { t as i18n } from '../lib/i18n.js';
  import X from 'lucide-svelte/icons/x';
  import { flip } from 'svelte/animate';
  import { gated, slideAway } from '../lib/motion.js';

  // The toasts that stay close up as one leaves, rather than jumping.
  const settle = gated(flip);

  function act(t) {
    dismissToast(t.id);
    t.action.run();
  }
</script>

<div class="toasts" role="status" aria-live="polite">
  {#each $toasts as t (t.id)}
    <div class="toast {t.kind}" class:has-action={t.action} out:slideAway animate:settle={{ duration: 200 }}>
      <span class="message">{t.message}</span>
      {#if t.action}
        <button class="btn small" on:click={() => act(t)}>{t.action.label}</button>
        <button class="btn small ghost icon" aria-label={$i18n('common.dismiss')} title={$i18n('common.dismiss')} on:click={() => dismissToast(t.id)}><X size={14} /></button>
      {/if}
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    bottom: 40px;
    right: 20px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    z-index: 100;
  }
  .toast {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 11px 16px;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    font-size: 0.88rem;
    max-width: 420px;
    box-shadow: var(--shadow);
    animation: slide-in 0.18s ease-out;
  }
  .toast.has-action {
    padding: 7px 8px 7px 16px;
  }
  .message {
    flex: 1;
    min-width: 0;
  }
  .toast.success {
    border-color: rgba(var(--success-rgb), 0.4);
  }
  .toast.error {
    border-color: rgba(var(--danger-rgb), 0.5);
  }
  .toast.warning {
    border-color: rgba(var(--warn-rgb), 0.55);
  }
  @keyframes slide-in {
    from {
      transform: translateX(30px);
      opacity: 0;
    }
    to {
      transform: none;
      opacity: 1;
    }
  }
</style>
