<script>
  import { backdropClose } from '../lib/backdrop.js';
  import ReleaseNotes from './ReleaseNotes.svelte';
  import DiscordBanner from './DiscordBanner.svelte';
  import { navigate } from '../lib/stores.js';
  import X from 'lucide-svelte/icons/x';

  export let releases = [];
  export let version = '';
  export let from = '';
  export let onClose = () => {};

  function onKeydown(e) {
    if (e.key === 'Escape') onClose();
  }

  function openFullChangelog() {
    onClose();
    navigate('changelog');
  }
</script>

<svelte:window on:keydown={onKeydown} />

<div class="backdrop" use:backdropClose={onClose} role="presentation">
  <div class="modal" role="dialog" aria-modal="true" aria-label="What's new in OpenSave">
    <button class="btn ghost icon x" on:click={onClose} title="Close" aria-label="Close"><X size={18} /></button>

    <div class="hero">
      <div class="badge">Updated</div>
      <h2>OpenSave {version}</h2>
      {#if from}<p class="from">You were on {from}</p>{/if}
    </div>

    <div class="body">
      <div class="banner-slot"><DiscordBanner compact /></div>
      <ReleaseNotes {releases} compact />
    </div>

    <footer>
      <button class="btn" on:click={openFullChangelog}>Full changelog</button>
      <button class="btn primary" on:click={onClose}>Got it</button>
    </footer>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: var(--overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 95;
    padding: 32px;
  }
  .modal {
    position: relative;
    width: min(560px, 100%);
    max-height: min(78vh, 720px);
    display: flex;
    flex-direction: column;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
    overflow: hidden;
  }
  .x {
    position: absolute;
    top: 12px;
    right: 12px;
    z-index: 1;
  }
  .hero {
    padding: 26px 28px 20px;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(180deg, rgba(var(--accent-rgb), 0.12), transparent);
  }
  .badge {
    display: inline-block;
    font-size: 0.66rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--accent);
    background: rgba(var(--accent-rgb), 0.14);
    border: 1px solid rgba(var(--accent-rgb), 0.3);
    border-radius: 999px;
    padding: 3px 10px;
    margin-bottom: 10px;
  }
  h2 {
    font-size: 1.4rem;
    font-weight: 700;
    letter-spacing: -0.02em;
  }
  .from {
    color: var(--text-faint);
    font-size: 0.82rem;
    margin-top: 3px;
  }
  .body {
    padding: 22px 28px;
    overflow-y: auto;
    flex: 1;
  }
  .banner-slot {
    margin-bottom: 20px;
  }
  footer {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    padding: 14px 20px;
    border-top: 1px solid var(--border);
    background: var(--bg);
  }
</style>
