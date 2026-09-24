<script>
  // The keyboard: listens for the shortcuts in lib/shortcuts.js and owns the
  // two things they open, the Ctrl+K palette and the list of shortcuts.
  import { navigate } from '../lib/stores.js';
  import { matchShortcut, paletteOpen, shortcutsOpen } from '../lib/shortcuts.js';
  import { contextMenu } from '../lib/contextmenu.js';
  import CommandPalette from './CommandPalette.svelte';
  import ShortcutsSheet from './ShortcutsSheet.svelte';

  // Ctrl+F finds in whatever is in front: a dialog's search box if a dialog
  // is open, else the page's, else the sidebar's library filter — so it
  // always lands somewhere, and never in a box hidden behind a dialog.
  function find() {
    const dialogs = document.querySelectorAll('.overlay, [role="dialog"]');
    const top = dialogs[dialogs.length - 1];
    const box = top ? top.querySelector('[data-find]') : document.querySelector('[data-find]') ?? document.querySelector('[data-find-fallback]');
    if (!box) return;
    box.focus();
    box.select?.();
  }

  function onKeydown(e) {
    const action = matchShortcut(e);
    if (!action) return;
    // The palette takes its own keys; a second Ctrl+K closes it.
    if ($paletteOpen && action !== 'palette') return;
    e.preventDefault();
    contextMenu.set(null);
    if (action === 'palette') paletteOpen.update((open) => !open);
    else if (action === 'help') shortcutsOpen.set(true);
    else if (action === 'find') find();
    else if (action.startsWith('go:')) navigate(action.slice(3));
  }
</script>

<svelte:window on:keydown={onKeydown} />

{#if $paletteOpen}
  <CommandPalette
    on:close={() => paletteOpen.set(false)}
    on:help={() => {
      paletteOpen.set(false);
      shortcutsOpen.set(true);
    }}
  />
{/if}
{#if $shortcutsOpen}
  <ShortcutsSheet onClose={() => shortcutsOpen.set(false)} />
{/if}
