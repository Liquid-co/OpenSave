<script>
  // Which events interrupt you on this device. See lib/notifyprefs.js.
  import { notifyPrefs, NOTIFY_EVENTS } from '../../lib/notifyprefs.js';
  import { exampleEvents, exampleEvent } from '../../lib/notifications.js';
  import { playChime } from '../../lib/notify.js';
  import { toast, gameList } from '../../lib/stores.js';
  import { native, gameCover } from '../../lib/api.js';
  import { t } from '../../lib/i18n.js';
  import BellRing from 'lucide-svelte/icons/bell-ring';

  // How each kind shows, played through once: a message in the corner with a
  // new entry behind the bell, then — a moment later — the chime that comes
  // with something that needs you (which also brings OpenSave to the front
  // when it is behind other windows), and last the notification the desktop
  // shows when OpenSave is not in front — sent here even though it is, so it
  // can be seen. It uses a game from the library, for its cover.
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
        open: JSON.stringify(game ? { view: 'game', params: { gameId: game.id } } : { view: 'settings', params: {} })
      });
      toast(why ? $t('settings.notifications.desktopFail', { why }) : $t('settings.notifications.desktopDemo'), why ? 'error' : 'info', { ttl: 9000 });
    }, 4800);
  }

  const toggle = (key) => notifyPrefs.update((p) => ({ ...p, [key]: !p[key] }));
  const attention = Object.entries(NOTIFY_EVENTS).filter(([, e]) => e.kind === 'attention');
  const messages = Object.entries(NOTIFY_EVENTS).filter(([, e]) => e.kind === 'message');
</script>

<div class="notif">
<div class="demo">
  <p class="hint">
    {$t('settings.notifications.bellHint')}
  </p>
  <button class="btn small" disabled={showing} on:click={showMe}><BellRing size={14} />{$t('settings.notifications.showMe')}</button>
</div>
<p class="lead">{$t('settings.notifications.chimeLead')}</p>
{#each attention as [key, e]}
  <label class="check">
    <input type="checkbox" checked={$notifyPrefs[key]} on:change={() => toggle(key)} />
    {e.label}
  </label>
{/each}

<p class="lead">{$t('settings.notifications.messageLead')}</p>
{#each messages as [key, e]}
  <label class="check">
    <input type="checkbox" checked={$notifyPrefs[key]} on:change={() => toggle(key)} />
    {e.label}
  </label>
{/each}

<label class="check quiet">
  <input type="checkbox" checked={$notifyPrefs.desktop} on:change={() => toggle('desktop')} />
  {$t('settings.notifications.desktopCheckbox')}
</label>
<p class="hint">
  {$t('settings.notifications.desktopHint')}
</p>

<label class="check quiet">
  <input type="checkbox" checked={$notifyPrefs.quietWhilePlaying} on:change={() => toggle('quietWhilePlaying')} />
  {$t('settings.notifications.quietCheckbox')}
</label>
<p class="hint">
  {$t('settings.notifications.quietHint')}
</p>
</div>

<style>
  /* Not .hint: a hint straight after a switch is pulled up to it (app.css),
     and these head the group below them instead. */
  .lead {
    margin: 14px 0 2px;
    font-size: 0.8rem;
    color: var(--text-faint);
  }
  .lead:first-child {
    margin-top: 0;
  }
  .quiet {
    margin-top: 12px;
  }
  .demo {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 12px;
    padding-bottom: 12px;
    border-bottom: 1px solid var(--border);
  }
  .demo .hint {
    margin: 0;
  }
</style>
