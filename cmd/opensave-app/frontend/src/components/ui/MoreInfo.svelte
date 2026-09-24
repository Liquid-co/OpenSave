<script>
  // The rest of a long hint, one click away. The first sentence of a hint
  // says what the setting is; the paragraph after it says why and when, and
  // printed in full under every field it turned the form into a wall of grey
  // text. Opens in place rather than as a hover bubble: a bubble clips at the
  // card's edge, and can't be reached from the keyboard.
  import Info from 'lucide-svelte/icons/info';

  let open = false;
</script>

<button type="button" class="more-toggle" aria-expanded={open} on:click|preventDefault={() => (open = !open)}>
  <Info size={12} strokeWidth={2.4} />{open ? 'Less' : 'More'}
</button>
{#if open}<span class="more-body"><slot /></span>{/if}

<style>
  .more-toggle {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    margin-left: 4px;
    padding: 0 5px;
    border: none;
    border-radius: 5px;
    background: var(--btn-bg);
    color: var(--text-dim);
    font: inherit;
    font-size: 0.74rem;
    line-height: 1.5;
    vertical-align: 1px;
    cursor: pointer;
  }
  .more-toggle:hover {
    color: var(--text);
    background: var(--btn-bg-hover);
  }
  .more-body {
    display: block;
    margin-top: 4px;
  }
</style>
