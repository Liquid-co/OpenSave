<script>
  // Track one folder by hand: a name and a path. Fires `close` on Cancel and
  // after tracking, which also opens the new game's page.
  import { createEventDispatcher } from 'svelte';
  import { navigate, toast } from '../../lib/stores.js';
  import { api, native } from '../../lib/api.js';

  const dispatch = createEventDispatcher();

  let name = '';
  let path = '';
  let adding = false;

  async function pickFolder() {
    const dir = await native.selectDirectory('Select the save folder to track');
    if (dir) path = dir;
  }

  async function add() {
    if (!name || !path || adding) return;
    adding = true;
    try {
      const game = await api.post('/api/games', { name, savePath: path });
      toast(`Now tracking "${name}"`, 'success');
      dispatch('close');
      navigate('game', { gameId: game.id });
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      adding = false;
    }
  }
</script>

<div class="card add">
  <h3>Track a save folder</h3>
  <div class="row">
    <div class="field grow">
      <label for="g-name">Game name</label>
      <input id="g-name" placeholder="e.g. Elden Ring" bind:value={name} on:keydown={(e) => e.key === 'Enter' && add()} />
    </div>
    <div class="field grow2">
      <label for="g-path">Save folder or file</label>
      <div class="path-row">
        <input id="g-path" placeholder="C:\Users\you\AppData\…" bind:value={path} on:keydown={(e) => e.key === 'Enter' && add()} />
        <button class="btn" on:click={pickFolder}>Browse</button>
      </div>
    </div>
  </div>
  <div class="actions">
    <button class="btn" on:click={() => dispatch('close')}>Cancel</button>
    <button class="btn primary" disabled={!name || !path || adding} on:click={add}>
      {adding ? 'Adding…' : 'Start tracking'}
    </button>
  </div>
</div>

<style>
  .add {
    margin-bottom: 20px;
  }
  .add h3 {
    margin-bottom: 14px;
  }
  .row {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
  }
  .grow {
    flex: 1;
    min-width: 180px;
  }
  .grow2 {
    flex: 2;
    min-width: 260px;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
