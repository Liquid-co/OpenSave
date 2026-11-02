<script>
  import { t as translateUI } from '../../lib/i18n.js';
  // Where a provider without a sign-in sends its backups.
  import { native } from '../../lib/api.js';

  export let config;

  async function pickFolder() {
    const dir = await native.selectDirectory($translateUI('providerFields.selectBackupDestinationFolder'));
    if (dir) config.url = dir;
  }
</script>

{#if config.provider === 'local'}
  <div class="field">
    <label for="cb-folder">{$translateUI('providerFields.destinationFolderEGANASMount')}</label>
    <div class="path-row">
      <input id="cb-folder" bind:value={config.url} placeholder="D:\Backups\OpenSave" />
      <button class="btn" on:click={pickFolder}>{$translateUI('providerFields.browse')}</button>
    </div>
  </div>
{:else if config.provider === 'webdav'}
  <div class="field">
    <label for="cb-url">{$translateUI('providerFields.webDAVURL')}</label>
    <input id="cb-url" bind:value={config.url} placeholder="https://nas.local/dav/opensave/" />
  </div>
  <div class="two">
    <div class="field">
      <label for="cb-user">{$translateUI('providerFields.username')}</label>
      <input id="cb-user" bind:value={config.username} />
    </div>
    <div class="field">
      <label for="cb-pass">{$translateUI('providerFields.password')}</label>
      <input id="cb-pass" type="password" bind:value={config.password} />
    </div>
  </div>
{:else if config.provider === 'webhook'}
  <div class="field">
    <label for="cb-hook">{$translateUI('providerFields.webhookURLReceivesMultipartPOST')}</label>
    <input id="cb-hook" bind:value={config.url} />
  </div>
  <div class="field">
    <label for="cb-headers">{$translateUI('providerFields.customHeadersJSON')}</label>
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
