<script>
  // ── Extra save locations ─────────────────────────────────────────
  //
  // A location learned from a peer or a backup arrives named but not placed:
  // the name travels between devices, the folder it lives in does not. Those
  // show as needing a folder, because until one is chosen the location is
  // silently skipped by every sync and every restore — and silence is exactly
  // what makes that dangerous.
  import { askConfirm } from '../../lib/stores.js';
  import { api, native } from '../../lib/api.js';

  export let game;
  export let runner;
  /** Read by the page: the locations besides the save folder. */
  export let locations = [];
  const { busy, run } = runner;

  let newLocation = '';

  async function load() {
    try {
      locations = (await api.get(`/api/games/${game.id}/roots`)) ?? [];
    } catch {
      locations = [];
    }
  }
  load();

  async function pick(name) {
    const label = String(name ?? '').trim();
    if (!label || $busy) return;
    const dir = await native.selectDirectory(`Folder for “${label}” — ${game.name}`);
    if (!dir) return;
    await run(`“${label}” now covered`, async () => {
      await api.post(`/api/games/${game.id}/roots`, { name: label, path: dir });
      newLocation = '';
      await load();
    });
  }

  async function remove(name) {
    if (
      !(await askConfirm(
        `Stop covering the “${name}” folder for ${game.name}? Its files are left exactly where they are — this only stops OpenSave syncing and snapshotting them.`,
        { title: 'Remove save location?', confirmText: 'Remove', danger: true }
      ))
    )
      return;
    await run(`Removed “${name}”`, async () => {
      await api.del(`/api/games/${game.id}/roots/${encodeURIComponent(name)}`);
      await load();
    });
  }
</script>

<div class="section">
  <h4>Save locations</h4>
  <p class="hint">
    Some games keep their save split across more than one folder — the save data in one
    place, settings or mods in another. Add each extra folder here and it is synced,
    snapshotted and restored along with the main one.
  </p>
  <div class="row">
    <span class="name">main save</span>
    <span class="path" title={game.savePath}>{game.savePath}</span>
  </div>
  {#each locations as loc (loc.name)}
    <div class="row" class:unmapped={!loc.mapped}>
      <span class="name">{loc.name}</span>
      {#if loc.mapped}
        <span class="path" title={loc.path}>{loc.path}</span>
      {:else}
        <span class="path missing">
          no folder on this device — this location isn't being synced
        </span>
      {/if}
      <button class="btn small" disabled={$busy} on:click={() => pick(loc.name)}>
        {loc.mapped ? 'Change' : 'Choose folder'}
      </button>
      <button class="btn small danger" disabled={$busy} on:click={() => remove(loc.name)}>
        Remove
      </button>
    </div>
  {/each}
  <div class="add">
    <input placeholder="Name for the folder (e.g. config)" bind:value={newLocation} />
    <button class="btn" disabled={!newLocation || $busy} on:click={() => pick(newLocation)}>
      + Add a folder
    </button>
  </div>
  <span class="hint">
    Give the same <strong>name</strong> on your other devices — that is what the two
    sides match on, since the folder lives somewhere different on each machine. Removing
    a location here never deletes its files.
  </span>
</div>

<style>
  .section {
    margin-top: 18px;
    padding-top: 16px;
    border-top: 1px solid var(--border);
  }
  h4 {
    font-size: 0.92rem;
    font-weight: 600;
    margin-bottom: 6px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px;
    margin-top: 8px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font-size: 0.84rem;
  }
  .row.unmapped {
    border-color: rgba(251, 191, 36, 0.4);
  }
  .name {
    flex: none;
    min-width: 90px;
    font-weight: 600;
  }
  .path {
    flex: 1;
    min-width: 0;
    color: var(--text-dim);
    font-size: 0.78rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .path.missing {
    color: var(--warn);
  }
  .add {
    display: flex;
    gap: 8px;
    margin-top: 10px;
  }
  .add input {
    flex: 1;
    padding: 8px 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.86rem;
    outline: none;
  }
</style>
