<script>
  // "Drop a folder to track it", while something is dragged over the window,
  // and the listener that takes the drop. See lib/filedrop.js.
  import { onDestroy, onMount } from 'svelte';
  import FolderPlus from 'lucide-svelte/icons/folder-plus';
  import { dragging, listenForDrops } from '../lib/filedrop.js';

  let stop = () => {};
  onMount(() => (stop = listenForDrops()));
  onDestroy(() => stop());

  // Only a drag carrying files, and only in the desktop app, which is where a
  // dropped folder can be read. Counted, because dragenter and dragleave fire
  // for every element the pointer crosses on the way.
  const canDrop = () => !!globalThis.runtime?.OnFileDrop;
  let depth = 0;
  const hasFiles = (e) => [...(e.dataTransfer?.types ?? [])].includes('Files');
  function enter(e) {
    if (!canDrop() || !hasFiles(e)) return;
    depth++;
    dragging.set(true);
  }
  function leave(e) {
    if (!canDrop() || !hasFiles(e)) return;
    depth = Math.max(0, depth - 1);
    if (depth === 0) dragging.set(false);
  }
  function over(e) {
    if (canDrop() && hasFiles(e)) e.preventDefault();
  }
  function dropped() {
    depth = 0;
    dragging.set(false);
  }
</script>

<svelte:window on:dragenter={enter} on:dragleave={leave} on:dragover={over} on:drop={dropped} />

{#if $dragging}
  <div class="drop">
    <div class="box">
      <FolderPlus size={34} strokeWidth={1.6} />
      <strong>Drop a save folder to track it</strong>
      <span>You'll get to check its name first.</span>
    </div>
  </div>
{/if}

<style>
  .drop {
    position: fixed;
    inset: 0;
    z-index: 300;
    display: grid;
    place-items: center;
    background: var(--overlay);
    pointer-events: none;
  }
  .box {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: 34px 44px;
    border: 2px dashed var(--accent);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    color: var(--accent);
  }
  strong {
    color: var(--text);
    font-size: 1.05rem;
  }
  span {
    color: var(--text-dim);
    font-size: 0.86rem;
  }
</style>
