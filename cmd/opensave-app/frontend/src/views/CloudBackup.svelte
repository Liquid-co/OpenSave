<script>
  import { gameList, toast } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';
  import { onMount } from 'svelte';

  let config = null;
  let busy = false;
  let authCode = '';
  let authInProgress = false;
  let selectedGame = '';
  let cloudFiles = null;

  const providers = [
    { id: 'local', label: 'Local folder', oauth: false },
    { id: 'webdav', label: 'WebDAV', oauth: false },
    { id: 'webhook', label: 'Webhook', oauth: false },
    { id: 'google_drive', label: 'Google Drive', oauth: true },
    { id: 'dropbox', label: 'Dropbox', oauth: true },
    { id: 'onedrive', label: 'OneDrive', oauth: true }
  ];

  onMount(load);

  async function load() {
    try {
      const settings = await api.get('/api/settings');
      config = settings.cloudSync ?? {
        enabled: false, provider: 'local', url: '', username: '', password: '', headers: '{}', folderId: ''
      };
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function save() {
    busy = true;
    try {
      await api.post('/api/settings', { cloudSync: config });
      toast('Cloud settings saved', 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  async function startAuth() {
    busy = true;
    try {
      const { authUrl } = await api.post('/api/auth/start', { provider: config.provider });
      native.openExternal(authUrl);
      authInProgress = true;
      toast('Sign in using the browser window, then paste the code from the redirect URL here.');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  async function finishAuth() {
    busy = true;
    try {
      const res = await api.post('/api/auth/callback', { code: authCode.trim() });
      toast(`Connected as ${res.userEmail}`, 'success');
      authInProgress = false;
      authCode = '';
      await load();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  async function disconnect() {
    busy = true;
    try {
      await api.post('/api/auth/disconnect');
      toast('Disconnected');
      await load();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  async function pickLocalFolder() {
    const dir = await native.selectDirectory('Select backup destination folder');
    if (dir) config.url = dir;
  }

  async function browseCloud() {
    if (!selectedGame) return;
    cloudFiles = null;
    try {
      cloudFiles = await api.get(`/api/cloud/snapshots/${selectedGame}`);
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  const restoreCloud = async (file) => {
    if (!confirm(`Restore ${file.snapshotId} from the cloud over your current save?`)) return;
    busy = true;
    try {
      await api.post(`/api/cloud/restore/${selectedGame}`, { fileName: file.name });
      toast('Restored from cloud', 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  };

  const uploadLocal = async () => {
    if (!selectedGame) return;
    busy = true;
    try {
      const res = await api.post(`/api/cloud/sync-local/${selectedGame}`);
      toast(`Uploaded ${res.uploaded}, skipped ${res.skipped}`, 'success');
      browseCloud();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  };

  // ── .sscb export/import ──────────────────────────────────────────
  const exportBackup = async () => {
    const target = await native.selectSaveFile('Export all snapshots', 'opensave-backup.sscb');
    if (!target) return;
    busy = true;
    try {
      const res = await api.post('/api/backup/export', { targetPath: target });
      toast(`Exported ${res.snapshotCount} snapshot(s)`, 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  };

  const importBackup = async () => {
    const src = await native.selectFile('Select an .sscb backup to import');
    if (!src) return;
    busy = true;
    try {
      const res = await api.post('/api/backup/restore', { sourcePath: src });
      toast(`Imported ${res.imported}, skipped ${res.skipped}`, 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  };

  $: currentProvider = providers.find((p) => p.id === config?.provider);
  const fmtSize = (n) => (n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : (n / 1024).toFixed(1) + ' KB');
</script>

<div class="head">
  <h2 class="page-title">Cloud Backup</h2>
</div>

{#if !config}
  <p class="quiet">Loading…</p>
{:else}
  <div class="card">
    <div class="enable-row">
      <div>
        <h3>Mirror snapshots to the cloud</h3>
        <p class="quiet">Every new snapshot uploads automatically in the background.</p>
      </div>
      <label class="switch">
        <input type="checkbox" bind:checked={config.enabled} on:change={save} />
        <span></span>
      </label>
    </div>

    <div class="pill-tabs" style="margin: 16px 0;">
      {#each providers as p}
        <button class:active={config.provider === p.id} on:click={() => { config.provider = p.id; cloudFiles = null; }}>
          {p.label}
        </button>
      {/each}
    </div>

    {#if config.provider === 'local'}
      <div class="field">
        <label for="cb-folder">Destination folder (e.g. a NAS mount)</label>
        <div class="path-row">
          <input id="cb-folder" bind:value={config.url} placeholder="D:\Backups\OpenSave" />
          <button class="btn" on:click={pickLocalFolder}>Browse</button>
        </div>
      </div>
    {:else if config.provider === 'webdav'}
      <div class="field">
        <label for="cb-url">WebDAV URL</label>
        <input id="cb-url" bind:value={config.url} placeholder="https://nas.local/dav/opensave/" />
      </div>
      <div class="two">
        <div class="field">
          <label for="cb-user">Username</label>
          <input id="cb-user" bind:value={config.username} />
        </div>
        <div class="field">
          <label for="cb-pass">Password</label>
          <input id="cb-pass" type="password" bind:value={config.password} />
        </div>
      </div>
    {:else if config.provider === 'webhook'}
      <div class="field">
        <label for="cb-hook">Webhook URL (receives multipart POST)</label>
        <input id="cb-hook" bind:value={config.url} />
      </div>
      <div class="field">
        <label for="cb-headers">Custom headers (JSON)</label>
        <input id="cb-headers" bind:value={config.headers} placeholder={'{"Authorization": "Bearer …"}'} />
      </div>
    {:else}
      <!-- OAuth providers -->
      {#if config.tokens?.userEmail}
        <div class="connected">
          <span class="badge online">connected</span>
          <span>{config.tokens.userEmail}</span>
          <button class="btn small danger" disabled={busy} on:click={disconnect}>Disconnect</button>
        </div>
      {:else}
        <button class="btn primary" disabled={busy} on:click={startAuth}>
          Sign in with {currentProvider?.label}
        </button>
        {#if authInProgress}
          <div class="auth-code">
            <p class="quiet">
              After approving access, the browser lands on a localhost page. Copy the <code>code</code> value
              from its address bar and paste it here:
            </p>
            <div class="path-row">
              <input placeholder="4/0AY0e-g7…" bind:value={authCode} />
              <button class="btn primary" disabled={!authCode || busy} on:click={finishAuth}>Connect</button>
            </div>
          </div>
        {/if}
      {/if}
      {#if config.provider === 'google_drive'}
        <div class="field" style="margin-top: 14px;">
          <label for="cb-folderid">Drive folder ID (optional)</label>
          <input id="cb-folderid" bind:value={config.folderId} />
        </div>
      {/if}
    {/if}

    <div class="actions">
      <button class="btn primary" disabled={busy} on:click={save}>Save settings</button>
    </div>
  </div>

  <h3 class="section">Browse cloud snapshots</h3>
  <div class="card">
    <div class="browse-row">
      <select bind:value={selectedGame}>
        <option value="">Select a game…</option>
        {#each $gameList as g}
          <option value={g.id}>{g.name}</option>
        {/each}
      </select>
      <button class="btn" disabled={!selectedGame} on:click={browseCloud}>List cloud snapshots</button>
      <button class="btn" disabled={!selectedGame || busy} on:click={uploadLocal}>Upload local snapshots</button>
    </div>
    {#if cloudFiles}
      {#if cloudFiles.length === 0}
        <p class="quiet" style="margin-top: 12px;">No cloud snapshots for this game yet.</p>
      {:else}
        <div class="cloud-list">
          {#each cloudFiles as f (f.name)}
            <div class="cloud-row">
              <div class="cloud-info">
                <div class="cloud-name">{f.snapshotId} <span class="badge offline">{f.branch}</span></div>
                <div class="cloud-meta">{fmtSize(f.sizeBytes)} · {new Date(f.createdTime).toLocaleString()}</div>
              </div>
              <button class="btn small primary" disabled={busy} on:click={() => restoreCloud(f)}>Restore</button>
            </div>
          {/each}
        </div>
      {/if}
    {/if}
  </div>

  <h3 class="section">Full backup file</h3>
  <div class="card export-row">
    <div>
      <h3>Export / import everything</h3>
      <p class="quiet">A single compressed .sscb file with every snapshot — for moving to a new PC.</p>
    </div>
    <div class="export-actions">
      <button class="btn" disabled={busy} on:click={importBackup}>Import .sscb</button>
      <button class="btn primary" disabled={busy} on:click={exportBackup}>Export all</button>
    </div>
  </div>
{/if}

<style>
  .head {
    margin-bottom: 20px;
  }
  .enable-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 16px;
  }
  .quiet {
    color: var(--text-faint);
    font-size: 0.85rem;
  }
  .switch {
    position: relative;
    width: 44px;
    height: 24px;
    flex-shrink: 0;
  }
  .switch input {
    opacity: 0;
    width: 0;
    height: 0;
  }
  .switch span {
    position: absolute;
    inset: 0;
    background: var(--bg-active);
    border-radius: 999px;
    cursor: pointer;
    transition: background 0.15s;
  }
  .switch span::before {
    content: '';
    position: absolute;
    width: 18px;
    height: 18px;
    left: 3px;
    top: 3px;
    background: var(--text-dim);
    border-radius: 50%;
    transition: transform 0.15s, background 0.15s;
  }
  .switch input:checked + span {
    background: var(--accent);
  }
  .switch input:checked + span::before {
    transform: translateX(20px);
    background: #fff;
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .path-row {
    display: flex;
    gap: 8px;
  }
  .path-row input {
    flex: 1;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 8px;
  }
  .connected {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .auth-code {
    margin-top: 12px;
  }
  .auth-code code {
    background: var(--bg);
    padding: 1px 5px;
    border-radius: 4px;
  }
  .section {
    margin: 22px 0 10px;
  }
  .browse-row {
    display: flex;
    gap: 10px;
  }
  .browse-row select {
    flex: 1;
    padding: 9px 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
    outline: none;
  }
  .cloud-list {
    margin-top: 12px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .cloud-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .cloud-info {
    flex: 1;
  }
  .cloud-name {
    font-weight: 600;
    font-size: 0.9rem;
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .cloud-meta {
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  .export-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 16px;
  }
  .export-actions {
    display: flex;
    gap: 8px;
  }
</style>
