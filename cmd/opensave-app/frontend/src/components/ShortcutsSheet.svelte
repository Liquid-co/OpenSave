<script>
  // "?": every keyboard shortcut, from the same list the handler uses.
  import Keyboard from 'lucide-svelte/icons/keyboard';
  import Modal from './ui/Modal.svelte';
  import { SHORTCUTS, PAGES } from '../lib/shortcuts.js';
  import { t as i18n } from '../lib/i18n.js';

  export let onClose = () => {};
  const isMac = /Mac/i.test(globalThis.navigator?.platform ?? '');
  const show = (keys) => (isMac ? keys.replace('Ctrl', '⌘') : keys);

  // The entries' wording is written in lib/shortcuts.js, in English; the same
  // entries are said here from their keys, translated. The Ctrl 1…6 entries
  // name pages, by the same keys the sidebar shows them by.
  const PAGE_LABEL_KEYS = {
    home: 'home.title',
    devices: 'home.devices',
    cloud: 'home.cloudBackup',
    activity: 'sidebar.nav.activity',
    settings: 'sidebar.nav.settings',
    changelog: 'about.changelog'
  };
  const WHAT = {
    'Ctrl K': 'shortcuts.jumpTo',
    'Ctrl F': 'shortcuts.searchPage',
    'Ctrl ,': 'shortcuts.openSettings',
    '← → ↑ ↓': 'shortcuts.moveGames',
    'Shift F10': 'shortcuts.actionsForGame',
    '?': 'shortcuts.thisList',
    Esc: 'shortcuts.closeDialog'
  };
  const what = (translate, keys) => {
    const n = /^Ctrl ([1-9])$/.exec(keys);
    if (n && PAGES[Number(n[1]) - 1]) {
      const page = PAGES[Number(n[1]) - 1];
      return translate('palette.goTo', { page: translate(PAGE_LABEL_KEYS[page.id]) });
    }
    return translate(WHAT[keys] ?? keys);
  };
</script>

<Modal title={$i18n('shortcuts.title')} icon={Keyboard} {onClose} width={520} height="auto">
  <dl>
    {#each SHORTCUTS as [keys]}
      <dt>{#each show(keys).split(' ') as k}<kbd>{k}</kbd>{/each}</dt>
      <dd>{what($i18n, keys)}</dd>
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
