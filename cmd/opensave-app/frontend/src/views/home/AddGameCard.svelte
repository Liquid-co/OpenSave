<script>
  // Naming a folder picked to track. "Track folder" opens the system's folder
  // picker straight away (Home.svelte), and this is what follows it — or a
  // folder dropped onto the window: the folder, a name to confirm, and a way
  // to pick another folder or a single file instead. The name is suggested
  // from the path (GET /api/suggest-name), which knows a Switch game by the
  // name its emulator shows. Fires `close` on Cancel and after tracking, which
  // also opens the new game's page.
  import { createEventDispatcher, onMount, tick } from 'svelte';
  import FolderPlus from 'lucide-svelte/icons/folder-plus';
  import Modal from '../../components/ui/Modal.svelte';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import { navigate, toast } from '../../lib/stores.js';
  import { api, native } from '../../lib/api.js';
  import { folderName } from '../../lib/filedrop.js';

  const dispatch = createEventDispatcher();

  /** The folder or file picked; '' where there is no picker (a browser). */
  export let path = '';

  let name = '';
  let typed = false; // once typed in, a late suggestion must not replace it
  let adding = false;
  let nameInput;
  const canPick = native.isWails();

  let suggestedFor = null;
  $: if (path !== suggestedFor) suggest(path);
  async function suggest(p) {
    suggestedFor = p;
    if (!p) return;
    let guess = folderName(p);
    try {
      guess = (await api.get(`/api/suggest-name?path=${encodeURIComponent(p)}`)).name || guess;
    } catch {
      // The folder's own name will do.
    }
    if (p !== path || typed) return;
    name = guess;
    await tick();
    nameInput?.select();
  }

  onMount(() => nameInput?.focus());

  async function pickFolder() {
    const dir = await native.selectDirectory('Choose the save folder to track');
    if (dir) {
      typed = false;
      path = dir;
    }
  }

  async function pickFile() {
    const file = await native.selectFile('Choose the save file to track');
    if (file) {
      typed = false;
      path = file;
    }
  }

  async function add() {
    if (!name.trim() || !path || adding) return;
    adding = true;
    try {
      const game = await api.post('/api/games', { name: name.trim(), savePath: path });
      toast(`Now tracking "${game.name ?? name}"`, 'success');
      dispatch('close');
      navigate('game', { gameId: game.id });
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      adding = false;
    }
  }
</script>

<Modal title="Track a game" icon={FolderPlus} width={560} height="auto" onClose={() => dispatch('close')}>
  <div class="body">
    <div class="field">
      <span class="label">Save folder or file</span>
      {#if canPick}
        <div class="picked" title={path}><bdi>{path}</bdi></div>
        <div class="alt">
          <button class="linklike" on:click={pickFolder}>Change folder…</button>
          <button class="linklike" on:click={pickFile}>Track a single file instead…</button>
        </div>
      {:else}
        <input id="g-path" placeholder="C:\Users\you\AppData\…" bind:value={path} />
      {/if}
    </div>
    <div class="field">
      <label for="g-name">Name</label>
      <input
        id="g-name"
        bind:this={nameInput}
        placeholder="e.g. Elden Ring"
        bind:value={name}
        on:input={() => (typed = true)}
        on:keydown={(e) => e.key === 'Enter' && add()}
      />
      <span class="hint">How it shows in your library, and on your other devices.</span>
    </div>
  </div>
  <ModalFoot>
    <span />
    <div class="actions">
      <button class="btn" on:click={() => dispatch('close')}>Cancel</button>
      <button class="btn primary" disabled={!name.trim() || !path || adding} on:click={add}>
        {adding ? 'Adding…' : 'Start tracking'}
      </button>
    </div>
  </ModalFoot>
</Modal>

<style>
  .body {
    padding: 18px 22px 20px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .label,
  label {
    font-size: 0.82rem;
    color: var(--text-dim);
  }
  .picked {
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: 0.8rem;
    color: var(--text);
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 8px 10px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl; /* long paths keep their end — the part that names the game */
    text-align: left;
  }
  .alt {
    display: flex;
    gap: 16px;
  }
  .linklike {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-size: 0.8rem;
    color: var(--text-dim);
    text-decoration: underline;
    cursor: pointer;
  }
  .linklike:hover {
    color: var(--text);
  }
  .hint {
    margin: 0;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
</style>
