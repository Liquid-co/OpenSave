<script>
  // A row of filter buttons, each with a count: "Games 12", "In cloud 3".
  /** [[id, label], …] in display order. */
  export let options = [];
  /** Count per id; an id with no entry shows none. */
  export let counts = {};
  export let value;
</script>

<div class="tabs">
  {#each options as [id, label]}
    <button class:active={value === id} on:click={() => (value = id)}>
      {label}
      {#if counts[id] !== undefined}<span class="count">{counts[id]}</span>{/if}
    </button>
  {/each}
</div>

<style>
  /* Pills, as on the Library: quiet until chosen. */
  .tabs {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
  }
  .tabs button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 30px;
    padding: 0 12px;
    border: 1px solid transparent;
    border-radius: 999px;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-md);
    white-space: nowrap;
    cursor: pointer;
  }
  .tabs button:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .tabs button.active {
    background: var(--bg-active);
    color: var(--text);
    border-color: var(--border-strong);
    font-weight: 600;
  }
  .count {
    color: var(--text-faint);
    font-size: var(--text-sm);
    font-weight: 400;
  }
</style>
