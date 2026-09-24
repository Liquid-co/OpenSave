<script>
  export let value = '';
  export let placeholder = 'Filter…';
</script>

<input class="search" {placeholder} bind:value />

<style>
  .search {
    flex: 1;
    min-width: 200px;
    padding: 9px 13px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
    outline: none;
  }
</style>
