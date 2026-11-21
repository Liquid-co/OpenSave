<script>
  // Cloud backup: whether it is on and where it goes. Setting it up, and
  // everything it holds, is on the Backups page; this says how it stands
  // and takes you there.
  import SettingGroup from '../../../components/ui/SettingGroup.svelte';
  import SettingRow from '../../../components/ui/SettingRow.svelte';
  import ArrowRight from 'lucide-svelte/icons/arrow-right';
  import { settings, navigate } from '../../../lib/stores.js';
  import { providerConfigured } from '../../../lib/cloudproviders.js';
  import { closeSettings } from '../../../lib/settingsmenu.js';
  import { t } from '../../../lib/i18n.js';

  $: cloud = $settings?.cloudSync ?? null;
  $: configured = !!cloud && providerConfigured(cloud);
  $: state = !configured ? $t('settings.cloud.notSetUp') : cloud.enabled ? $t('settings.cloud.on', { provider: $t('cloud.provider.' + cloud.provider) }) : $t('backups.hero.off');
</script>

<SettingGroup>
  <SettingRow id="cloudBackup" label={state} hint={$t('settings.sync.cloudBackupHint')}>
    <button
      class="btn small"
      on:click={() => {
        closeSettings();
        navigate('cloud');
      }}>{$t('settings.cloud.open')}<ArrowRight size={14} /></button
    >
  </SettingRow>
</SettingGroup>
