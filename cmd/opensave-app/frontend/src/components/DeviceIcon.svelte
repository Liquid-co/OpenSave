<script>
  // A paired or discovered device, by kind: a handheld or a computer.
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Monitor from 'lucide-svelte/icons/monitor';

  /** The device's reported type; 'deck' is a handheld. */
  export let type = '';
</script>

<span class="device-icon" title={type === 'deck' ? 'Handheld' : 'Computer'}>
  <svelte:component this={type === 'deck' ? Gamepad2 : Monitor} size={19} />
</span>

<style>
  .device-icon {
    flex-shrink: 0;
    width: 38px;
    height: 38px;
    border-radius: 10px;
    display: grid;
    place-items: center;
    background: var(--bg-hover);
    color: var(--text-dim);
  }
</style>
