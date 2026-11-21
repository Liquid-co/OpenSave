<script>
  // Using OpenSave from a gamepad: on, off, or when one is in use, and what
  // its buttons do. See views/settings/ControllerOptions.svelte.
  import SettingGroup from '../../../components/ui/SettingGroup.svelte';
  import SettingRow from '../../../components/ui/SettingRow.svelte';
  import ControllerOptions from '../ControllerOptions.svelte';
  import { t } from '../../../lib/i18n.js';
</script>

<SettingGroup>
  <SettingRow id="controller" label={$t('settings.general.controller')} under>
    <ControllerOptions />
  </SettingRow>
</SettingGroup>
