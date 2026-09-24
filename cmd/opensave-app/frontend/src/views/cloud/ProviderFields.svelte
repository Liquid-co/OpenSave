<script>
  // Where a provider without a sign-in sends its backups.
  import { native } from '../../lib/api.js';

  export let config;

  async function pickFolder() {
    const dir = await native.selectDirectory('Select backup destination folder');
    if (dir) config.url = dir;
  }
</script>

{#if config.provider === 'local'}
  <div class="field">
    <label for="cb-folder">Destination folder (e.g. a NAS mount)</label>
    <div class="path-row">
      <input id="cb-folder" bind:value={config.url} placeholder="D:\Backups\OpenSave" />
      <button class="btn" on:click={pickFolder}>Browse</button>
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
{/if}

<style>
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
</style>
