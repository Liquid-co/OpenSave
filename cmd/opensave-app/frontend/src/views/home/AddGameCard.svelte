<script>
  // Naming a folder picked to track. "Track folder" opens the system's folder
  // picker straight away (Home.svelte), and this is what follows it — or a
  // folder dropped onto the window: the folder, a name to confirm, and a way
  // to pick another folder or a single file instead. The name is suggested
  // from the path (GET /api/suggest-name), which knows a Switch game by the
  // name its emulator shows. Fires `close` on Cancel and after tracking, which
  // also opens the new game's page.
  //
  // The browser build has no system picker, so there the path is typed or
  // pasted, or chosen with Browse (FolderBrowser).
  import { createEventDispatcher, onMount, tick } from 'svelte';
  import FolderPlus from 'lucide-svelte/icons/folder-plus';
  import Folder from 'lucide-svelte/icons/folder';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import FileText from 'lucide-svelte/icons/file-text';
  import BadgeCheck from 'lucide-svelte/icons/badge-check';
  import Modal from '../../components/ui/Modal.svelte';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import FolderBrowser from '../../components/FolderBrowser.svelte';
  import { navigate, toast } from '../../lib/stores.js';
  import { api, native } from '../../lib/api.js';
  import { folderName } from '../../lib/filedrop.js';
  import { t } from '../../lib/i18n.js';

  const dispatch = createEventDispatcher();

  /** The folder or file picked; '' where there is no picker (a browser). */
  export let path = '';

  let name = '';
  // The game the folder is a save location of, in the game database: its
  // name and App ID (cover art, launching).
  let appId = '';
  let knownAs = '';
  let typed = false; // once typed in, a late suggestion must not replace it
  let adding = false;
  let nameInput;
  const canPick = native.isWails();
  let browsing = false;
  function browsed(e) {
    typed = false;
    path = e.detail;
    browsing = false;
  }

  let suggestedFor = null;
  $: if (path !== suggestedFor) suggest(path);
  async function suggest(p) {
    suggestedFor = p;
    if (!p) return;
    let guess = folderName(p);
    appId = '';
    knownAs = '';
    try {
      const id = await api.get(`/api/gamedb/identify?path=${encodeURIComponent(p)}`);
      if (id.found && p === path) {
        appId = id.game.appId ?? '';
        knownAs = id.game.name;
        guess = id.game.name;
      }
    } catch {
      // Named from the path instead.
    }
    if (!knownAs) {
      try {
        guess = (await api.get(`/api/suggest-name?path=${encodeURIComponent(p)}`)).name || guess;
      } catch {
        // The folder's own name will do.
      }
    }
    if (p !== path || typed) return;
    name = guess;
    await tick();
    nameInput?.select();
  }

  onMount(() => nameInput?.focus());

  async function pickFolder() {
    const dir = await native.selectDirectory($t('home.chooseFolderDialog'));
    if (dir) {
      typed = false;
      path = dir;
    }
  }

  async function pickFile() {
    const file = await native.selectFile($t('home.addGame.chooseFile'));
    if (file) {
      typed = false;
      path = file;
    }
  }

  async function add() {
    if (!name.trim() || !path || adding) return;
    adding = true;
    try {
      const game = await api.post('/api/games', { name: name.trim(), savePath: path, appId });
      toast($t('common.nowTracking', { name: game.name ?? name }), 'success');
      dispatch('close');
      navigate('game', { gameId: game.id });
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      adding = false;
    }
  }
</script>

<Modal title={$t('home.addGame.title')} icon={FolderPlus} width={560} height="auto" onClose={() => dispatch('close')}>
  <div class="body">
    <div class="field">
      <span class="label">{$t('home.addGame.savePath')}</span>
      {#if canPick}
        <div class="folder">
          <span class="ftile"><Folder size={18} /></span>
          <div class="picked" title={path}><bdi>{path}</bdi></div>
        </div>
        <div class="alt">
          <button class="mini" on:click={pickFolder}><FolderOpen size={14} />{$t('home.addGame.changeFolder')}</button>
          <button class="mini" on:click={pickFile}><FileText size={14} />{$t('home.addGame.trackFileInstead')}</button>
        </div>
      {:else}
        <div class="typed">
          <input id="g-path" placeholder="C:\Users\you\AppData\…" bind:value={path} />
          <button class="btn browse" class:on={browsing} aria-expanded={browsing} on:click={() => (browsing = !browsing)}><FolderOpen size={15} />{$t('folders.browse')}</button>
        </div>
        {#if browsing}<FolderBrowser start={path} on:pick={browsed} />{/if}
      {/if}
    </div>
    <div class="field">
      <label for="g-name">{$t('home.addGame.name')}</label>
      <input
        id="g-name"
        bind:this={nameInput}
        placeholder={$t('home.addGame.namePlaceholder')}
        bind:value={name}
        on:input={() => (typed = true)}
        on:keydown={(e) => e.key === 'Enter' && add()}
      />
      <span class="hint">{$t('home.addGame.nameHint')}</span>
      {#if knownAs}<span class="known"><BadgeCheck size={14} />{$t('home.addGame.recognisedAs', { name: knownAs })}</span>{/if}
    </div>
  </div>
  <ModalFoot>
    <span />
    <div class="actions">
      <button class="btn" on:click={() => dispatch('close')}>{$t('common.cancel')}</button>
      <button class="btn primary" disabled={!name.trim() || !path || adding} on:click={add}>
        {adding ? $t('home.addGame.adding') : $t('home.addGame.startTracking')}
      </button>
    </div>
  </ModalFoot>
</Modal>

<style>
  .body {
    padding: 20px 22px 22px;
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 7px;
    margin: 0;
  }
  .label,
  label {
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  /* A folder or program picked: its icon, then the path, the end of which
     names the game, so a long one keeps its end. */
  .folder {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 12px 8px 8px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 12px;
    min-width: 0;
  }
  .ftile {
    width: 34px;
    height: 34px;
    flex-shrink: 0;
    border-radius: 9px;
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .picked {
    flex: 1;
    min-width: 0;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: var(--text-sm);
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl;
    text-align: left;
  }
  .alt {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .mini {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 28px;
    padding: 0 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
    transition:
      background 0.12s,
      color 0.12s,
      border-color 0.12s;
  }
  .mini:hover {
    background: var(--bg-hover);
    border-color: var(--border-strong);
    color: var(--text);
  }
  .known {
    align-self: flex-start;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--text);
    font-size: var(--text-sm);
    animation: known-in 0.22s cubic-bezier(0.2, 0.7, 0.2, 1.3) backwards;
  }
  .known :global(svg) {
    color: var(--accent-text);
  }
  @keyframes known-in {
    from {
      opacity: 0;
      transform: scale(0.9);
    }
  }
  .hint {
    margin: 0;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
  .typed {
    display: flex;
    gap: 8px;
  }
  .typed input {
    flex: 1;
    min-width: 0;
  }
  .browse {
    flex-shrink: 0;
  }
  .browse.on {
    border-color: var(--accent);
    color: var(--accent-text);
  }
</style>
