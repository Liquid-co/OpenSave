<script>
  // The shape of a page while its data is on the way, in place of the word
  // "Loading": the eye settles on where things will be, and nothing jumps
  // when they arrive.
  //
  // 'tiles' is the library, 'cards' a settings-style stack, 'lines' a block
  // of text.
  export let kind = 'cards';
  export let count = 3;
</script>

<div class="skel {kind}" aria-busy="true" aria-label="Loading">
  {#if kind === 'tiles'}
    <div class="skeleton bar" style="width: 100%; height: 64px; margin-bottom: 22px; border-radius: var(--radius-lg)"></div>
    <div class="grid">
      {#each Array(count) as _}
        <div class="tile">
          <div class="skeleton art"></div>
          <div class="skeleton line" style="width: 55%"></div>
          <div class="skeleton line thin" style="width: 38%"></div>
        </div>
      {/each}
    </div>
  {:else if kind === 'lines'}
    {#each Array(count) as _, i}
      <div class="skeleton line" style="width: {[92, 78, 85, 60, 88, 70][i % 6]}%"></div>
    {/each}
  {:else}
    {#each Array(count) as _}
      <div class="card">
        <div class="skeleton line head" style="width: 28%"></div>
        <div class="skeleton field"></div>
        <div class="skeleton line thin" style="width: 64%"></div>
      </div>
    {/each}
  {/if}
</div>

<style>
  .skel {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .skel.lines {
    gap: 10px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: 14px;
  }
  .tile {
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    overflow: hidden;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding-bottom: 14px;
    background: var(--bg-raised);
  }
  .art {
    aspect-ratio: 460 / 215;
    border-radius: 0;
  }
  .tile .line {
    margin: 0 14px;
  }
  .line {
    height: 12px;
  }
  .line.thin {
    height: 9px;
  }
  .line.head {
    height: 15px;
    margin-bottom: 16px;
  }
  .field {
    height: 36px;
    margin-bottom: 10px;
    border-radius: 9px;
  }
</style>
