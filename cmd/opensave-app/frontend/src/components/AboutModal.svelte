<script>
  import { onMount } from 'svelte';
  import { native } from '../lib/api.js';
  import logoUrl from '../assets/logo.svg';

  export let onClose = () => {};

  let info = null;
  onMount(async () => {
    try {
      info = await native.appInfo();
    } catch {
      info = { name: 'OpenSave', version: '2.0.0' };
    }
  });

  function onKeydown(e) {
    if (e.key === 'Escape') onClose();
  }
</script>

<svelte:window on:keydown={onKeydown} />

<div class="backdrop" on:click|self={onClose} role="presentation">
  <div class="modal" role="dialog" aria-modal="true" aria-label="About OpenSave">
    <button class="x" on:click={onClose} title="Close" aria-label="Close">✕</button>
    <img class="logo" src={logoUrl} alt="" />
    <h2>{info?.name ?? 'OpenSave'}</h2>
    <div class="ver">Version {info?.version ?? '—'}</div>
    <p class="tagline">{info?.tagline ?? ''}</p>

    <div class="meta">
      <div><span>License</span> {info?.license ?? 'MIT'}</div>
      <div><span>Built with</span> {info?.tech ?? 'Go + Wails'}</div>
    </div>

    <p class="copy">{info?.copyright ?? ''}</p>
    <p class="note">Wire-compatible with the original Node.js/Electron OpenSave — Go and JS devices sync together.</p>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.62);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 90;
    padding: 32px;
  }
  .modal {
    position: relative;
    width: min(420px, 100%);
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    padding: 32px 28px 26px;
    text-align: center;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
  }
  .x {
    position: absolute;
    top: 12px;
    right: 12px;
    border: none;
    background: transparent;
    color: var(--text-faint);
    font-size: 0.9rem;
    cursor: pointer;
    padding: 4px 8px;
    border-radius: 6px;
  }
  .x:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .logo {
    width: 72px;
    height: 72px;
    border-radius: 18px;
    margin-bottom: 12px;
  }
  h2 {
    font-size: 1.4rem;
    font-weight: 700;
  }
  .ver {
    color: var(--accent);
    font-weight: 600;
    font-size: 0.9rem;
    margin-top: 2px;
  }
  .tagline {
    color: var(--text-dim);
    font-size: 0.9rem;
    margin-top: 8px;
  }
  .meta {
    display: flex;
    justify-content: center;
    gap: 22px;
    margin: 18px 0 14px;
    font-size: 0.85rem;
    color: var(--text);
  }
  .meta span {
    display: block;
    color: var(--text-faint);
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 2px;
  }
  .copy {
    font-size: 0.78rem;
    color: var(--text-faint);
  }
  .note {
    font-size: 0.76rem;
    color: var(--text-faint);
    margin-top: 12px;
    line-height: 1.5;
    border-top: 1px solid var(--border);
    padding-top: 12px;
  }
</style>
