<script>
  import Search from 'lucide-svelte/icons/search';
  export let value = '';
  export let placeholder = 'Filter…';
</script>

<!-- data-find: what Ctrl+F focuses (see components/Shortcuts.svelte). -->
<label class="search">
  <Search size={15} />
  <input data-find {placeholder} aria-label={placeholder} bind:value />
</label>

<style>
  .search {
    flex: 1;
    min-width: 200px;
    display: flex;
    align-items: center;
    gap: 9px;
    height: 36px;
    padding: 0 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: calc(10px * var(--round, 1));
    color: var(--text-faint);
    cursor: text;
    transition:
      border-color 0.12s,
      box-shadow 0.12s;
  }
  .search:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px rgba(var(--accent-rgb), 0.16);
    color: var(--text-dim);
  }
  input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-base);
  }
</style>
