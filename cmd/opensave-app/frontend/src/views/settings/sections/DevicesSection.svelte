<script>
  // Devices: how your devices reach each other over the internet — the
  // relay this device uses, and hosting one here. Pairing and each device's
  // own settings are on the Devices page.
  import SettingGroup from '../../../components/ui/SettingGroup.svelte';
  import SettingRow from '../../../components/ui/SettingRow.svelte';
  import Switch from '../../../components/ui/Switch.svelte';
  import RadioTower from 'lucide-svelte/icons/radio-tower';
  import ArrowRight from 'lucide-svelte/icons/arrow-right';
  import { api } from '../../../lib/api.js';
  import { navigate, toast } from '../../../lib/stores.js';
  import { closeSettings } from '../../../lib/settingsmenu.js';
  import { collapse } from '../../../lib/motion.js';
  import { t, segments } from '../../../lib/i18n.js';

  export let draft;
  export let unsaved = [];
  export let save = () => {};

  // This device's addresses, for someone joining the relay hosted here.
  // Shown only when asked for: they identify the machine, and should not
  // be on screen just because hosting is on. Fetched each time — a public
  // address changes, and a stale one sent to a friend is worse than none.
  let info = null;
  let shown = false;
  let loading = false;
  async function toggleAddresses() {
    if (shown) {
      shown = false;
      return;
    }
    loading = true;
    try {
      info = await api.get('/api/relay/ips');
      shown = true;
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      loading = false;
    }
  }
  $: if (!draft.hostRelay && shown) shown = false;
</script>

<SettingGroup title={$t('settings.sync.relay')}>
  <SettingRow id="relayUrl" label={$t('settings.sync.relayUrl')} control="s-relay-url">
    <svelte:fragment slot="hint">
      {#each segments($t('settings.sync.relayUrlHint'), { internetSync: $t('settings.sync.internetSync') }, ['internetSync']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
    </svelte:fragment>
    <input id="s-relay-url" class="wide" class:invalid={unsaved.includes('relayUrl')} bind:value={draft.relayUrl} placeholder="wss://relay.opensave.org" spellcheck="false" />
  </SettingRow>
  <SettingRow id="hostRelay" label={$t('settings.sync.hostRelay')} hint={$t('settings.sync.hostRelayHint')}>
    <Switch
      checked={draft.hostRelay}
      label={$t('settings.sync.hostRelay')}
      on:change={(e) => {
        draft.hostRelay = e.detail;
        save();
      }}
    />
  </SettingRow>
  {#if draft.hostRelay}
    <div transition:collapse>
      <SettingRow id="relayPort" label={$t('settings.sync.relayPort')} hint={$t('settings.sync.relayPortHint')} control="s-relay-port">
        <input id="s-relay-port" type="number" class="num" class:invalid={unsaved.includes('relayPort')} bind:value={draft.relayPort} />
        <button class="btn small" on:click={toggleAddresses} disabled={loading}>
          {loading ? $t('settings.sync.lookingUp') : shown ? $t('settings.sync.hideAddresses') : $t('settings.sync.showAddresses')}
        </button>
      </SettingRow>
      {#if shown && info}
        <div class="share" transition:collapse>
          <div class="share-title"><RadioTower size={15} />{$t('settings.sync.shareTitle')}</div>
          <div class="share-row"><span>{$t('settings.sync.lanIps')}</span><code>{info.lanIps?.join(', ') || '—'}</code></div>
          <div class="share-row"><span>{$t('settings.sync.publicIp')}</span><code>{info.publicIp || $t('settings.sync.unavailable')}</code></div>
          <div class="share-row"><span>{$t('settings.sync.relayPortLabel')}</span><code>{info.relayPort}</code></div>
        </div>
      {/if}
    </div>
  {/if}
</SettingGroup>

<SettingGroup>
  <SettingRow label={$t('settings.section.devices')} hint={$t('settings.section.devicesHint')}>
    <button
      class="btn small"
      on:click={() => {
        closeSettings();
        navigate('devices');
      }}>{$t('settings.devices.open')}<ArrowRight size={14} /></button
    >
  </SettingRow>
</SettingGroup>

<style>
  .wide {
    width: 280px;
    max-width: 100%;
  }
  .num {
    width: 110px;
  }
  .invalid {
    border-color: var(--danger);
  }
  .share {
    margin: 0 18px 14px;
    padding: 12px 14px;
    border-radius: 10px;
    background: var(--bg);
    border: 1px solid var(--border);
    font-size: var(--text-md);
  }
  .share-title {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
    font-weight: 600;
  }
  .share-row {
    display: flex;
    gap: 12px;
    padding: 3px 0;
  }
  .share-row span {
    width: 130px;
    color: var(--text-dim);
  }
</style>
