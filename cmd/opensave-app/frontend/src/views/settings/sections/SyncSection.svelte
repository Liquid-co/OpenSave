<script>
  // Sync: what syncs on its own, which games a new device's saves count as,
  // and how fast saves may move.
  import Select from '../../../components/ui/Select.svelte';
  import SettingGroup from '../../../components/ui/SettingGroup.svelte';
  import SettingRow from '../../../components/ui/SettingRow.svelte';
  import Switch from '../../../components/ui/Switch.svelte';
  import MoreInfo from '../../../components/ui/MoreInfo.svelte';
  import { t, segments } from '../../../lib/i18n.js';

  export let draft;
  export let save = () => {};

  const flip = (key) => (e) => {
    draft[key] = e.detail;
    save();
  };
</script>

<SettingGroup>
  <SettingRow id="autoSyncOnTrack" label={$t('settings.sync.autoSyncOnTrack')}>
    <Switch checked={draft.autoSyncOnTrack} label={$t('settings.sync.autoSyncOnTrack')} on:change={flip('autoSyncOnTrack')} />
  </SettingRow>
  <SettingRow id="detectNewGames" label={$t('settings.sync.detectNewGames')} hint={$t('settings.sync.detectNewGamesHint')}>
    <Switch checked={draft.detectNewGames} label={$t('settings.sync.detectNewGames')} on:change={flip('detectNewGames')} />
  </SettingRow>
  <SettingRow id="matchByAppId" label={$t('settings.sync.matchByAppId')}>
    <svelte:fragment slot="hint">
      {$t('settings.sync.matchByAppIdHint')}
      <MoreInfo>{$t('settings.sync.matchByAppIdMore')}</MoreInfo>
    </svelte:fragment>
    <Switch checked={draft.matchByAppId} label={$t('settings.sync.matchByAppId')} on:change={flip('matchByAppId')} />
  </SettingRow>
</SettingGroup>

<SettingGroup>
  <SettingRow id="unknownGame" label={$t('settings.sync.unknownGameLabel')} control="s-unknown-game">
    <svelte:fragment slot="hint">
      {$t('settings.sync.unknownGameHint')}
      <MoreInfo>
        {#each segments($t('settings.sync.unknownGameMore'), { askMe: $t('settings.sync.askShort') }, ['askMe']) as part}{#if part.bold}<em>{part.text}</em>{:else}{part.text}{/if}{/each}
      </MoreInfo>
    </svelte:fragment>
    <Select
      id="s-unknown-game"
      bind:value={draft.unknownGameFromPeer}
      options={[
        { value: 'track', label: $t('settings.sync.unknownGame.track') },
        { value: 'ask', label: $t('settings.sync.unknownGame.ask') }
      ]}
    />
  </SettingRow>
  <SettingRow id="speedLimit" label={$t('settings.sync.bandwidthLimit')} hint={$t('settings.sync.speedHint')} control="s-limit">
    <Select
      id="s-limit"
      bind:value={draft.speedLimit}
      options={[
        [0, 'unlimited'],
        [100, 'low'],
        [500, 'medium'],
        [1024, 'high'],
        [5120, 'veryHigh'],
        [10240, 'ultra']
      ].map(([value, k]) => ({ value, label: $t('settings.sync.speed.' + k) }))}
    />
  </SettingRow>
</SettingGroup>
