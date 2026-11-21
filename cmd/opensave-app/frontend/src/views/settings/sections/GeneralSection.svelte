<script>
  // General: this device — its language, name, kind and id — and how
  // OpenSave starts and updates.
  import Select from '../../../components/ui/Select.svelte';
  import SettingGroup from '../../../components/ui/SettingGroup.svelte';
  import SettingRow from '../../../components/ui/SettingRow.svelte';
  import Switch from '../../../components/ui/Switch.svelte';
  import Copy from 'lucide-svelte/icons/copy';
  import Check from 'lucide-svelte/icons/check';
  import { locale, LOCALES, t, segments, SYSTEM_LOCALE } from '../../../lib/i18n.js';
  import { showSetupAgain } from '../../../lib/setup.js';
  import { navigate } from '../../../lib/stores.js';
  import { closeSettings } from '../../../lib/settingsmenu.js';

  export let draft;
  export let unsaved = [];
  export let save = () => {};
  export let appVersion = '';

  $: onPreRelease = appVersion.includes('-');

  let copied = false;
  async function copyId() {
    try {
      await navigator.clipboard?.writeText(draft.nodeId ?? '');
      copied = true;
      setTimeout(() => (copied = false), 1600);
    } catch {}
  }
  const flip = (key) => (e) => {
    draft[key] = e.detail;
    save();
  };
</script>

<SettingGroup title={$t('settings.general.deviceIdentity')}>
  <SettingRow id="deviceName" label={$t('settings.general.deviceName')} control="s-name">
    <input id="s-name" class="wide" class:invalid={unsaved.includes('deviceName')} bind:value={draft.deviceName} />
  </SettingRow>
  <SettingRow id="deviceType" label={$t('settings.general.deviceType')} hint={$t('settings.general.typeHint')} control="s-type">
    <Select
      id="s-type"
      bind:value={draft.deviceType}
      options={['desktop', 'deck', 'handheld', 'mobile'].map((v) => ({ value: v, label: $t('settings.general.type.' + v) }))}
    />
  </SettingRow>
  <SettingRow id="deviceId" label={$t('settings.general.deviceId')} hint={$t('settings.general.deviceIdHint')}>
    <code class="id" title={draft.nodeId}>{draft.nodeId ?? ''}</code>
    <button class="icon-btn" title={$t('common.copy')} aria-label={$t('common.copy')} on:click={copyId}
      >{#if copied}<Check size={15} />{:else}<Copy size={15} />{/if}</button
    >
  </SettingRow>
  <SettingRow id="language" label={$t('settings.language')} control="s-lang">
    <Select
      id="s-lang"
      value={$locale}
      options={[{ value: SYSTEM_LOCALE, label: $t('settings.systemDefault') }, ...Object.entries(LOCALES).map(([value, label]) => ({ value, label }))]}
      on:change={(e) => locale.set(e.detail)}
    />
  </SettingRow>
</SettingGroup>

<SettingGroup title={$t('settings.general.startup')}>
  <SettingRow id="startOnBoot" label={$t('settings.general.startOnBoot')} hint={$t('settings.general.startOnBootHint')}>
    <Switch checked={draft.startOnBoot} label={$t('settings.general.startOnBoot')} on:change={flip('startOnBoot')} />
  </SettingRow>
</SettingGroup>

<SettingGroup title={$t('settings.general.updates')}>
  <SettingRow id="betas" label={$t('settings.general.offerBetas')}>
    <svelte:fragment slot="hint">
      {#if onPreRelease}
        {#each segments($t('settings.general.betasOnPreRelease'), { version: appVersion }, ['version']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
      {:else}
        {$t('settings.general.betasStable')}
      {/if}
    </svelte:fragment>
    <Switch
      checked={draft.updateChannel === 'beta'}
      label={$t('settings.general.offerBetas')}
      on:change={(e) => {
        draft.updateChannel = e.detail ? 'beta' : 'stable';
        save();
      }}
    />
  </SettingRow>
  <SettingRow id="setupGuide" label={$t('settings.general.setupGuide')} hint={$t('settings.general.setupGuideHint')}>
    <button
      class="btn small"
      on:click={() => {
        showSetupAgain();
        closeSettings();
        navigate('home');
      }}>{$t('settings.general.showSetupAgain')}</button
    >
  </SettingRow>
</SettingGroup>

<style>
  .wide {
    width: 260px;
    max-width: 100%;
  }
  .invalid {
    border-color: var(--danger);
  }
  .id {
    max-width: 220px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-sm);
    color: var(--text-dim);
  }
  .icon-btn {
    width: 30px;
    height: 30px;
    display: grid;
    place-items: center;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg);
    color: var(--text-dim);
    cursor: pointer;
  }
  .icon-btn:hover {
    color: var(--text);
    border-color: var(--border-strong);
  }
</style>
