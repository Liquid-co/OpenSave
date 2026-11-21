<script>
  // A paired or discovered device, by kind: a handheld or a computer.
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Monitor from 'lucide-svelte/icons/monitor';
  import { isHandheld } from '../lib/devices.js';

  /** The device's reported type. */
  export let type = '';

  $: handheld = isHandheld(type);
</script>

<span class="device-icon" title={handheld ? 'Handheld' : 'Computer'}>
  <svelte:component this={handheld ? Gamepad2 : Monitor} size={19} />
</span>

<style>
  .device-icon {
    flex-shrink: 0;
    width: 38px;
    height: 38px;
    border-radius: calc(10px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--bg-hover);
    color: var(--text-dim);
  }
</style>
