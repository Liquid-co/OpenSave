<script>
  import { settings, toast } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';

  let tab = 'general';
  let draft = null;
  let busy = false;

  $: if ($settings && !draft) draft = structuredClone($settings);

  async function save() {
    busy = true;
    try {
      const updated = await api.post('/api/settings', draft);
      settings.set(updated);
      draft = structuredClone(updated);
      toast('Settings saved', 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  // Path translations editor
  function addRule() {
    draft.pathTranslations = [...(draft.pathTranslations ?? []), { fromPattern: '', toPattern: '' }];
  }
  function removeRule(i) {
    draft.pathTranslations = draft.pathTranslations.filter((_, idx) => idx !== i);
  }

  // Custom scan paths
  async function addScanPath() {
    const dir = await native.selectDirectory('Add a folder to auto-scan');
    if (dir) draft.customScanPaths = [...(draft.customScanPaths ?? []), dir];
  }
  function removeScanPath(i) {
    draft.customScanPaths = draft.customScanPaths.filter((_, idx) => idx !== i);
  }

  async function pickBackupsDir() {
    const dir = await native.selectDirectory('Select snapshots storage folder');
    if (dir) draft.backupsDir = dir;
  }
</script>

<div class="head">
  <h2 class="page-title">Settings</h2>
</div>

{#if !draft}
  <p class="quiet">Loading…</p>
{:else}
  <div class="pill-tabs" style="margin-bottom: 18px;">
    <button class:active={tab === 'general'} on:click={() => (tab = 'general')}>General</button>
    <button class:active={tab === 'sync'} on:click={() => (tab = 'sync')}>Sync</button>
    <button class:active={tab === 'storage'} on:click={() => (tab = 'storage')}>Storage</button>
    <button class:active={tab === 'advanced'} on:click={() => (tab = 'advanced')}>Advanced</button>
  </div>

  {#if tab === 'general'}
    <div class="card">
      <div class="field">
        <label for="s-name">Device name — how other devices see you</label>
        <input id="s-name" bind:value={draft.deviceName} />
      </div>
      <div class="field">
        <label for="s-type">Device type</label>
        <select id="s-type" bind:value={draft.deviceType}>
          <option value="desktop">Desktop / laptop</option>
          <option value="deck">Steam Deck / handheld</option>
        </select>
      </div>
      <label class="check">
        <input type="checkbox" bind:checked={draft.startOnBoot} />
        Start OpenSave when the computer starts
      </label>
    </div>
  {:else if tab === 'sync'}
    <div class="card">
      <label class="check">
        <input type="checkbox" bind:checked={draft.autoSyncOnTrack} />
        Sync a game immediately when it's first tracked
      </label>
      <div class="field" style="margin-top: 14px;">
        <label for="s-limit">Internet upload/download limit (KB/s, 0 = unlimited)</label>
        <input id="s-limit" type="number" min="0" bind:value={draft.speedLimit} />
        <span class="hint">Only applies to relay (internet) syncs — LAN is never throttled.</span>
      </div>
    </div>
  {:else if tab === 'storage'}
    <div class="card">
      <div class="field">
        <label for="s-backups">Snapshots folder</label>
        <div class="path-row">
          <input id="s-backups" bind:value={draft.backupsDir} />
          <button class="btn" on:click={pickBackupsDir}>Browse</button>
        </div>
      </div>
      <label class="check">
        <input type="checkbox" bind:checked={draft.autoDeleteBackups} />
        Auto-delete old sync backups
      </label>
      {#if draft.autoDeleteBackups}
        <div class="field" style="margin-top: 10px;">
          <label for="s-days">Delete backups older than (days)</label>
          <input id="s-days" type="number" min="1" bind:value={draft.autoDeleteDays} />
        </div>
      {/if}

      <div class="field" style="margin-top: 16px;">
        <label for="s-scan-paths">Extra folders to auto-scan</label>
        {#each draft.customScanPaths ?? [] as p, i}
          <div class="rule-row">
            <span class="rule-path" title={p}>{p}</span>
            <button class="btn small danger" on:click={() => removeScanPath(i)}>✕</button>
          </div>
        {/each}
        <button id="s-scan-paths" class="btn small" on:click={addScanPath}>+ Add folder</button>
      </div>
    </div>
  {:else}
    <div class="card">
      <div class="field">
        <label for="s-port">Daemon port (restart required)</label>
        <input id="s-port" type="number" bind:value={draft.port} />
      </div>

      <div class="field">
        <label for="s-rules">Cross-platform path translation rules</label>
        <span class="hint">
          Rewrites a peer's save paths to local conventions, e.g. "C:\Users\me\Saves" → "/home/deck/saves".
        </span>
        {#each draft.pathTranslations ?? [] as rule, i}
          <div class="rule-row">
            <input placeholder="From pattern" bind:value={rule.fromPattern} />
            <span class="arrow">→</span>
            <input placeholder="To pattern" bind:value={rule.toPattern} />
            <button class="btn small danger" on:click={() => removeRule(i)}>✕</button>
          </div>
        {/each}
        <button id="s-rules" class="btn small" on:click={addRule}>+ Add rule</button>
      </div>
    </div>
  {/if}

  <div class="save-bar">
    <button class="btn primary" disabled={busy} on:click={save}>Save changes</button>
  </div>
{/if}

<style>
  .head {
    margin-bottom: 18px;
  }
  .quiet {
    color: var(--text-faint);
  }
  .check {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 0.92rem;
    color: var(--text);
    cursor: pointer;
    padding: 4px 0;
  }
  .check input {
    accent-color: var(--accent);
    width: 16px;
    height: 16px;
  }
  .path-row {
    display: flex;
    gap: 8px;
  }
  .path-row input {
    flex: 1;
  }
  .rule-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }
  .rule-row input {
    flex: 1;
    padding: 7px 10px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.85rem;
    outline: none;
  }
  .rule-path {
    flex: 1;
    font-size: 0.83rem;
    color: var(--text-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .arrow {
    color: var(--text-faint);
  }
  .save-bar {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
  }
</style>
