<script>
  // The arrow in front of something that expands: pointing right when shut,
  // down when open, turning between the two.
  import ChevronRight from 'lucide-svelte/icons/chevron-right';

  export let open = false;
  export let size = 14;
</script>

<span class="chevron" class:open aria-hidden="true"><ChevronRight {size} strokeWidth={2.4} /></span>

<style>
  .chevron {
    display: inline-flex;
    vertical-align: -2px;
    transition: transform 0.15s ease;
  }
  .chevron.open {
    transform: rotate(90deg);
  }
</style>
