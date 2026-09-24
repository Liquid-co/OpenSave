<script>
  // The provider cards. The one holding the stored sign-in keeps its tick
  // while you look at the others.
  import { providers, providerStatus } from '../../lib/cloudproviders.js';
  import ProviderIcon from './ProviderIcon.svelte';
  import Check from 'lucide-svelte/icons/check';

  export let config;
  export let connected = null;
</script>

<div class="label">Select cloud storage provider</div>
<div class="grid">
  {#each providers as p}
    <button class="card-btn" class:active={config.provider === p.id} on:click={() => (config.provider = p.id)}>
      {#if p.id === connected}
        <span class="tick" title="Connected"><Check size={13} strokeWidth={3.2} /></span>
      {/if}
      <div class="icon"><ProviderIcon provider={p} /></div>
      <div class="name">{p.label}</div>
      <div class="status" class:is-connected={p.id === connected}>
        {providerStatus(p.id, connected, config)}
      </div>
    </button>
  {/each}
</div>

<style>
  .label {
    display: block;
    font-size: 0.85rem;
    color: var(--text-dim);
    font-weight: 600;
    margin: 0 0 10px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 10px;
    margin-bottom: 18px;
  }
  .card-btn {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 7px;
    padding: 16px 12px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    color: var(--text-dim);
    cursor: pointer;
    transition: border-color 0.12s, background 0.12s, transform 0.12s;
    text-align: center;
  }
  .card-btn:hover {
    border-color: var(--border-strong);
    transform: translateY(-1px);
  }
  .card-btn.active {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .icon {
    width: 40px;
    height: 40px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .name {
    font-weight: 600;
    font-size: 0.9rem;
    color: var(--text);
  }
  .status {
    font-size: 0.72rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 130px;
  }
  .card-btn.active .status {
    color: var(--accent);
  }
  .status.is-connected,
  .card-btn.active .status.is-connected {
    color: var(--success);
    font-weight: 600;
  }
  .tick {
    position: absolute;
    top: 8px;
    right: 8px;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: var(--success);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
  }
</style>
