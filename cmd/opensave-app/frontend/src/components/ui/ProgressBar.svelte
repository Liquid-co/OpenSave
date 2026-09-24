<script>
  // "Uploading 3 of 12 — Elden Ring", with a bar under it. The label is the
  // slot; `done` and `total` size the bar, and a total of 0 (not known yet)
  // shows a sliver rather than an empty track.
  import Spinner from './Spinner.svelte';

  export let done = 0;
  export let total = 0;

  $: pct = total > 0 ? Math.round((done / total) * 100) : 8;
</script>

<div class="progress">
  <div class="label"><Spinner size={16} /> <slot /></div>
  <div class="bar"><div class="fill" style="width: {pct}%"></div></div>
</div>

<style>
  .progress {
    padding: 12px 22px;
    border-bottom: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .label {
    display: flex;
    align-items: center;
    gap: 9px;
    font-size: 0.84rem;
    color: var(--text-dim);
  }
  .label :global(code) {
    background: var(--bg);
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.78rem;
  }
  .bar {
    height: 6px;
    border-radius: 999px;
    background: var(--bg-active);
    overflow: hidden;
  }
  .fill {
    height: 100%;
    border-radius: 999px;
    background: var(--accent);
    transition: width 0.3s ease;
  }
</style>
