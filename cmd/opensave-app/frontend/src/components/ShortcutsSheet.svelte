<script>
  // "?": every keyboard shortcut, from the same list the handler uses.
  import Keyboard from 'lucide-svelte/icons/keyboard';
  import Modal from './ui/Modal.svelte';
  import { SHORTCUTS } from '../lib/shortcuts.js';

  export let onClose = () => {};
  const isMac = /Mac/i.test(globalThis.navigator?.platform ?? '');
  const show = (keys) => (isMac ? keys.replace('Ctrl', '⌘') : keys);
</script>

<Modal title="Keyboard shortcuts" icon={Keyboard} {onClose} width={520} height="auto">
  <dl>
    {#each SHORTCUTS as [keys, what]}
      <dt>{#each show(keys).split(' ') as k}<kbd>{k}</kbd>{/each}</dt>
      <dd>{what}</dd>
    {/each}
  </dl>
</Modal>

<style>
  dl {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 10px 18px;
    padding: 18px 22px 22px;
    align-items: center;
  }
  dt {
    display: flex;
    gap: 4px;
  }
  dd {
    color: var(--text-dim);
    font-size: 0.9rem;
  }
  kbd {
    min-width: 24px;
    text-align: center;
    font: inherit;
    font-size: 0.78rem;
    font-weight: 600;
    padding: 3px 7px;
    border: 1px solid var(--border-strong);
    border-bottom-width: 2px;
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
  }
</style>
