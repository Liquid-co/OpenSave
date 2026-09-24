<script>
  // The small ring shown while something loads. It was drawn three times over,
  // at three sizes, in three views' styles.
  export let size = 14;
</script>

<span class="spin" style="--size: {size}px"></span>

<style>
  .spin {
    display: inline-block;
    vertical-align: middle;
    flex-shrink: 0;
    width: var(--size);
    height: var(--size);
    border: 2px solid var(--accent-soft);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
