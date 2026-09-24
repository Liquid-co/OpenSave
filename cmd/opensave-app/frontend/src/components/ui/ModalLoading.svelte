<script>
  // Fills a Modal's body while its contents are being fetched.
  import Spinner from './Spinner.svelte';
</script>

<div class="loading"><Spinner size={16} /> <slot /></div>

<style>
  .loading {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    color: var(--text-dim);
  }
</style>
