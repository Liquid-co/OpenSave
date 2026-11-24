<script>
  // Where a provider without a sign-in sends its backups.
  import { native } from '../../lib/api.js';
  import { t } from '../../lib/i18n.js';

  export let config;

  async function pickFolder() {
    const dir = await native.selectDirectory($t('cloud.fields.selectFolderDialog'));
    if (dir) config.url = dir;
  }
</script>

{#if config.provider === 'local'}
  <div class="field">
    <label for="cb-folder">{$t('cloud.fields.destFolder')}</label>
    <div class="path-row">
      <input id="cb-folder" bind:value={config.url} placeholder="D:\Backups\OpenSave" />
      <button class="btn" on:click={pickFolder}>{$t('game.header.browse')}</button>
    </div>
  </div>
{:else if config.provider === 'webdav'}
  <div class="field">
    <label for="cb-url">{$t('cloud.fields.webdavUrl')}</label>
    <input id="cb-url" bind:value={config.url} placeholder="https://nas.local/dav/opensave/" />
  </div>
  <div class="two">
    <div class="field">
      <label for="cb-user">{$t('cloud.fields.username')}</label>
      <input id="cb-user" bind:value={config.username} />
    </div>
    <div class="field">
      <label for="cb-pass">{$t('cloud.fields.password')}</label>
      <input id="cb-pass" type="password" bind:value={config.password} />
    </div>
  </div>
{:else if config.provider === 'webhook'}
  <div class="field">
    <label for="cb-hook">{$t('cloud.fields.webhookUrl')}</label>
    <input id="cb-hook" bind:value={config.url} />
  </div>
  <div class="field">
    <label for="cb-headers">{$t('cloud.fields.customHeaders')}</label>
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
