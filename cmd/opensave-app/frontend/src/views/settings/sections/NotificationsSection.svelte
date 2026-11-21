<script>
  // Notifications on this device: which events chime and come to the front,
  // which only leave a message, the desktop's own notifications, and quiet
  // while a game is running. See lib/notifyprefs.js.
  import SettingGroup from '../../../components/ui/SettingGroup.svelte';
  import SettingRow from '../../../components/ui/SettingRow.svelte';
  import Switch from '../../../components/ui/Switch.svelte';
  import BellRing from 'lucide-svelte/icons/bell-ring';
  import { notifyPrefs, NOTIFY_EVENTS } from '../../../lib/notifyprefs.js';
  import { exampleEvents, exampleEvent } from '../../../lib/notifications.js';
  import { playChime } from '../../../lib/notify.js';
  import { toast, gameList } from '../../../lib/stores.js';
  import { native, gameCover } from '../../../lib/api.js';
  import { t } from '../../../lib/i18n.js';

  // How each kind shows, played through once: a message with a new entry
  // behind the bell, then — a moment later — the chime that comes with
  // something that needs you, and last the desktop's own notification, sent
  // even though OpenSave is in front so it can be seen. It uses a game from
  // the library, for its cover.
  let showing = false;
  function showMe() {
    showing = true;
    toast($t('settings.notifications.demoToast'), 'info', { action: { label: $t('settings.notifications.demoAction'), run: () => {} } });
    exampleEvents.update((list) => [exampleEvent(), ...list]);
    setTimeout(() => {
      playChime();
      toast($t('settings.notifications.chimeToast'), 'warning', { ttl: 9000 });
    }, 2200);
    setTimeout(async () => {
      showing = false;
      if (!$notifyPrefs.desktop) return;
      const game = $gameList[0];
      const why = await native.desktopNotify({
        title: game?.name ?? 'Hades',
        body: $t('settings.notifications.demoBody'),
        image: game ? gameCover(game) : '',
        open: JSON.stringify(game ? { view: 'game', params: { gameId: game.id } } : { view: 'settings', params: { section: 'notifications' } })
      });
      toast(why ? $t('settings.notifications.desktopFail', { why }) : $t('settings.notifications.desktopDemo'), why ? 'error' : 'info', { ttl: 9000 });
    }, 4800);
  }

  const setPref = (key) => (e) => notifyPrefs.update((p) => ({ ...p, [key]: e.detail }));
  const attention = Object.keys(NOTIFY_EVENTS).filter((k) => NOTIFY_EVENTS[k].kind === 'attention');
  const messages = Object.keys(NOTIFY_EVENTS).filter((k) => NOTIFY_EVENTS[k].kind === 'message');
</script>

<SettingGroup>
  <SettingRow id="notifications" label={$t('settings.general.notifications')} hint={$t('settings.notifications.bellHint')}>
    <button class="btn small" disabled={showing} on:click={showMe}><BellRing size={14} />{$t('settings.notifications.showMe')}</button>
  </SettingRow>
</SettingGroup>

<SettingGroup title={$t('settings.notifications.chimeLead')}>
  {#each attention as key}
    <SettingRow label={$t('notifyPrefs.event.' + key)}>
      <Switch checked={$notifyPrefs[key]} label={$t('notifyPrefs.event.' + key)} on:change={setPref(key)} />
    </SettingRow>
  {/each}
</SettingGroup>

<SettingGroup title={$t('settings.notifications.messageLead')}>
  {#each messages as key}
    <SettingRow label={$t('notifyPrefs.event.' + key)}>
      <Switch checked={$notifyPrefs[key]} label={$t('notifyPrefs.event.' + key)} on:change={setPref(key)} />
    </SettingRow>
  {/each}
</SettingGroup>

<SettingGroup>
  <SettingRow id="desktopNotes" label={$t('settings.notifications.desktopCheckbox')} hint={$t('settings.notifications.desktopHint')}>
    <Switch checked={$notifyPrefs.desktop} label={$t('settings.notifications.desktopCheckbox')} on:change={setPref('desktop')} />
  </SettingRow>
  <SettingRow id="quietWhilePlaying" label={$t('settings.notifications.quietCheckbox')} hint={$t('settings.notifications.quietHint')}>
    <Switch checked={$notifyPrefs.quietWhilePlaying} label={$t('settings.notifications.quietCheckbox')} on:change={setPref('quietWhilePlaying')} />
  </SettingRow>
</SettingGroup>
