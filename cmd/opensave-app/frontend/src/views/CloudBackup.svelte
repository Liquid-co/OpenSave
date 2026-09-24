<script>
  import { writable } from 'svelte/store';
  import { onMount } from 'svelte';
  import { toast, settings } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';
  import { connectedProviderOf, isOAuth } from '../lib/cloudproviders.js';
  import ProviderPicker from './cloud/ProviderPicker.svelte';
  import ProviderFields from './cloud/ProviderFields.svelte';
  import OAuthConnect from './cloud/OAuthConnect.svelte';
  import OwnOAuthApp from './cloud/OwnOAuthApp.svelte';
  import CloudBrowser from './cloud/CloudBrowser.svelte';
  import ExportDialog from './cloud/ExportDialog.svelte';
  import ImportDialog from './cloud/ImportDialog.svelte';

  let config = null;
  // Shared by everything on the page that talks to the provider, so one
  // change is in flight at a time.
  const busy = writable(false);

  // The provider the stored OAuth tokens belong to (server truth at load
  // time) — independent of which card is currently selected, so the
  // connected card keeps saying "connected" while you browse the others.
  let connected = null;

  onMount(() => load());

  /** Reads the settings. `keepProvider` keeps that card selected rather than
   *  the one last saved, for a change made to a card not yet saved. */
  async function load(keepProvider) {
    try {
      const s = await api.get('/api/settings');
      const next = s.cloudSync ?? {
        enabled: false, provider: 'local', url: '', username: '', password: '', headers: '{}', folderId: ''
      };
      connected = connectedProviderOf(next);
      if (keepProvider) next.provider = keepProvider;
      config = next;
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function save() {
    busy.set(true);
    try {
      settings.set(await api.post('/api/settings', { cloudSync: config }));
      toast('Cloud settings saved', 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy.set(false);
    }
  }

  let browserOpen = false;
  let exportOpen = false;
  let importSource = '';

  async function pickImportFile() {
    const src = await native.selectBackupFile('Select an .sscb backup to import');
    if (src) importSource = src;
  }
</script>

<div class="head">
  <h2 class="page-title">Cloud Backup</h2>
</div>

{#if !config}
  <p class="quiet">Loading…</p>
{:else}
  <div class="card">
    <ProviderPicker bind:config {connected} />
    <ProviderFields bind:config />
    <OAuthConnect {config} {connected} {busy} reload={() => load()} />
    {#if isOAuth(config.provider)}
      <!-- Rebuilt per provider, so one provider's credentials never show
           under another's name. -->
      {#key config.provider}
        <OwnOAuthApp {config} {connected} {busy} reload={load} />
      {/key}
    {/if}

    <div class="actions">
      <span class="quiet" style="margin-right: auto;">
        Automatic mirroring and the Drive folder ID live in <strong>Settings → Sync</strong>.
      </span>
      <button class="btn primary" disabled={$busy} on:click={save}>Save settings</button>
    </div>
  </div>

  <h3 class="section">Cloud snapshots</h3>
  <div class="card entry">
    <div>
      <h3>Browse your cloud library</h3>
      <p class="quiet">
        Every game with snapshots in the cloud, as cover-art tiles — upload, restore, or delete per game.
      </p>
    </div>
    <button class="btn primary" on:click={() => (browserOpen = true)}>☁️ Browse cloud</button>
  </div>

  <h3 class="section">Backup file (.sscb)</h3>
  <div class="card entry">
    <div>
      <h3>Export / import saves</h3>
      <p class="quiet">
        Pick any saves on this machine — tracked or just detected — and export them with their
        locations into one file. Import adds them to snapshots, or fully restores them onto disk.
      </p>
    </div>
    <div class="entry-actions">
      <button class="btn" disabled={$busy} on:click={pickImportFile}>Import .sscb</button>
      <button class="btn primary" disabled={$busy} on:click={() => (exportOpen = true)}>Export saves…</button>
    </div>
  </div>
{/if}

{#if exportOpen}
  <ExportDialog on:close={() => (exportOpen = false)} />
{/if}

{#if importSource}
  <ImportDialog source={importSource} on:close={() => (importSource = '')} />
{/if}

{#if browserOpen}
  <CloudBrowser {busy} onAuthLost={() => load()} on:close={() => (browserOpen = false)} />
{/if}

<style>
  .head {
    margin-bottom: 20px;
  }
  .quiet {
    color: var(--text-faint);
    font-size: 0.85rem;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 12px;
    margin-top: 8px;
  }
  .section {
    margin: 22px 0 10px;
  }
  .entry {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 16px;
  }
  .entry-actions {
    display: flex;
    gap: 8px;
  }
</style>
