<script>
  // What the cloud does by itself. The first switch is the whole of cloud
  // backup: off, nothing goes up and the copies already there are not read
  // (cloud.Service.config refuses everything while it is off), so it says so.
  import { createEventDispatcher } from 'svelte';
  import Switch from '../../components/ui/Switch.svelte';
  import { t } from '../../lib/i18n.js';

  export let enabled = true;
  export let autoPull = false;
  const dispatch = createEventDispatcher();
</script>

<section class="card auto" aria-labelledby="auto-title">
  <h2 id="auto-title">{$t('cloud.backup.automatically')}</h2>
  <div class="set">
    <div class="what">
      <div class="name" id="auto-up">{$t('backups.auto.upload')}</div>
      <div class="cap">{enabled ? $t('backups.auto.uploadSub') : $t('backups.auto.uploadOff')}</div>
    </div>
    <Switch checked={enabled} label={$t('backups.auto.upload')} on:change={(e) => dispatch('enabled', e.detail)} />
  </div>
  <div class="set">
    <div class="what">
      <div class="name">{$t('backups.auto.pull')}</div>
      <div class="cap">{$t('backups.auto.pullSub')}</div>
    </div>
    <Switch checked={autoPull} disabled={!enabled} label={$t('backups.auto.pull')} on:change={(e) => dispatch('autoPull', e.detail)} />
  </div>
  <p class="keeps">{$t('backups.auto.keeps')}</p>
</section>

<style>
  .auto {
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
    background: var(--bg-raised);
  }
  h2 {
    padding: 14px 16px 6px;
    font-size: var(--text-base);
    font-weight: 600;
  }
  .set {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 12px 16px;
    border-top: 1px solid var(--border);
  }
  .set:first-of-type {
    border-top: 0;
  }
  .what {
    flex: 1;
    min-width: 0;
  }
  .name {
    font-size: var(--text-md);
    font-weight: 600;
  }
  .cap {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .keeps {
    padding: 12px 16px 14px;
    border-top: 1px solid var(--border);
    font-size: var(--text-sm);
    color: var(--text-faint);
    line-height: 1.45;
  }
</style>
