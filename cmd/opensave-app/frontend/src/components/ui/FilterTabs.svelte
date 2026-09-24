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
  .tabs {
    display: flex;
    gap: 6px;
  }
  .tabs button {
    padding: 7px 13px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: transparent;
    color: var(--text-dim);
    font-size: 0.85rem;
    cursor: pointer;
  }
  .tabs button:hover {
    background: var(--bg-hover);
  }
  .tabs button.active {
    background: var(--bg-active);
    color: var(--text);
    border-color: var(--border-strong);
  }
  .count {
    color: var(--text-faint);
    font-size: 0.75rem;
    margin-left: 2px;
  }
</style>
