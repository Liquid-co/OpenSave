<script>
  // Settings that belong together, in one rounded panel, rows ruled apart
  // (SettingRow). A title above it when the section holds more than one.
  export let title = '';
</script>

<section class="sgroup">
  {#if title}<h3>{title}</h3>{/if}
  <div class="rows"><slot /></div>
</section>

<style>
  .sgroup {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  h3 {
    padding: 0 4px;
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  .rows {
    border-radius: calc(12px * var(--round, 1));
    background: var(--bg-raised);
    border: 1px solid var(--border);
  }
</style>
