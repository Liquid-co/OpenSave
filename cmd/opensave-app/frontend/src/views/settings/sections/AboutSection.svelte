<script>
  // About: which OpenSave this is and what is new in it, where to talk to the
  // people who make it, and how to keep it going.
  import SettingGroup from '../../../components/ui/SettingGroup.svelte';
  import SettingRow from '../../../components/ui/SettingRow.svelte';
  import Heart from 'lucide-svelte/icons/heart';
  import Globe from 'lucide-svelte/icons/globe';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Wrench from 'lucide-svelte/icons/wrench';
  import ArrowUpRight from 'lucide-svelte/icons/arrow-up-right';
  import ArrowRight from 'lucide-svelte/icons/arrow-right';
  import qrcode from 'qrcode-generator';
  import { native } from '../../../lib/api.js';
  import { navigate } from '../../../lib/stores.js';
  import { closeSettings } from '../../../lib/settingsmenu.js';
  import { DISCORD_URL, DONATE_URL } from '../../../lib/links.js';
  import { t } from '../../../lib/i18n.js';

  export let appVersion = '';

  // The donation page as a QR code, made here — no request leaves the app —
  // so paying from a phone, a tap there, needs no typing. Level M forgives
  // a little screen glare.
  const qrSvg = (() => {
    const qr = qrcode(0, 'M');
    qr.addData(DONATE_URL);
    qr.make();
    return qr.createSvgTag({ cellSize: 4, margin: 0, scalable: true });
  })();
  let donateOpened = false;
  function openDonate() {
    native.openExternal(DONATE_URL);
    donateOpened = true;
  }
</script>

<SettingGroup>
  <SettingRow id="version" label={$t('settings.about.version')}>
    <svelte:fragment slot="hint"><code>{appVersion || '—'}</code></svelte:fragment>
    <button
      class="btn small"
      on:click={() => {
        closeSettings();
        navigate('changelog');
      }}>{$t('sidebar.nav.whatsNew')}<ArrowRight size={14} /></button
    >
  </SettingRow>
  <SettingRow id="discord" label={$t('settings.about.discord')} hint={$t('settings.about.discordHint')}>
    <button class="btn small" on:click={() => native.openExternal(DISCORD_URL)}>{$t('settings.about.open')}<ArrowUpRight size={14} /></button>
  </SettingRow>
</SettingGroup>

<section class="support" id="set-support">
  <div class="hero">
    <span class="badge-heart"><Heart size={22} /></span>
    <div>
      <h3>{$t('settings.support.title')}</h3>
      <p class="lede">{$t('settings.support.lede')}</p>
    </div>
  </div>
  <div class="split">
    <div class="left">
      <p class="body">{$t('settings.support.body')}</p>
      <ul class="points">
        <li><span class="pt"><Globe size={15} /></span>{$t('settings.support.point1')}</li>
        <li><span class="pt"><Gamepad2 size={15} /></span>{$t('settings.support.point2')}</li>
        <li><span class="pt"><Wrench size={15} /></span>{$t('settings.support.point3')}</li>
      </ul>
      <div class="actions">
        <button class="btn primary" on:click={openDonate}>{donateOpened ? $t('settings.support.openAgain') : $t('settings.support.openDonate')}<ArrowUpRight size={15} /></button>
        {#if donateOpened}<span class="opened">{$t('settings.support.openedToast')}</span>{/if}
      </div>
      <p class="foot">{$t('settings.support.foot')}</p>
    </div>
    <div class="qr">
      <!-- Dark on a light plate even in the dark theme: phone cameras read
           an inverted code badly. -->
      <div class="plate">{@html qrSvg}</div>
      <span class="caption">{$t('settings.support.scanToPay')}<br />{$t('settings.support.fromPhone')}</span>
    </div>
  </div>
  <p class="note">{$t('settings.support.note')}</p>
</section>

<style>
  code {
    font-size: var(--text-md);
  }
  .support {
    padding: 18px;
    border-radius: calc(12px * var(--round, 1));
    background: var(--bg-raised);
    border: 1px solid var(--border);
  }
  .hero {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-bottom: 18px;
    padding: 16px 18px;
    border-radius: calc(12px * var(--round, 1));
    background:
      radial-gradient(120% 160% at 0% 0%, var(--accent-soft), transparent 62%),
      var(--bg);
    border: 1px solid var(--border);
  }
  .badge-heart {
    flex: none;
    width: 46px;
    height: 46px;
    display: grid;
    place-items: center;
    color: var(--accent-text);
    border-radius: 50%;
    background: var(--accent-soft);
    border: 1px solid rgba(var(--accent-rgb), 0.35);
  }
  h3 {
    margin: 0 0 4px;
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .lede,
  .body {
    color: var(--text-dim);
    font-size: var(--text-md);
    line-height: 1.5;
  }
  .body {
    margin-bottom: 14px;
    max-width: 56ch;
  }
  .split {
    display: flex;
    gap: 28px;
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .left {
    flex: 1;
    min-width: 240px;
  }
  .points {
    list-style: none;
    margin: 0 0 18px;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 9px;
    font-size: var(--text-md);
  }
  .points li {
    display: flex;
    align-items: center;
    gap: 11px;
  }
  .pt {
    flex: none;
    width: 28px;
    height: 28px;
    display: grid;
    place-items: center;
    color: var(--text-dim);
    border-radius: calc(8px * var(--round, 1));
    background: var(--bg-hover);
    border: 1px solid var(--border);
  }
  .actions {
    display: flex;
    gap: 12px;
    align-items: center;
    flex-wrap: wrap;
  }
  .opened {
    font-size: var(--text-sm);
    color: var(--text-dim);
  }
  .foot {
    margin-top: 14px;
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .qr {
    flex: none;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 14px;
    border-radius: calc(12px * var(--round, 1));
    background: var(--bg);
    border: 1px solid var(--border);
  }
  .plate {
    background: #fff;
    padding: 10px;
    border-radius: calc(10px * var(--round, 1));
    line-height: 0;
  }
  .plate :global(svg) {
    display: block;
    width: 116px;
    height: 116px;
    shape-rendering: crispEdges;
  }
  .caption {
    font-size: var(--text-xs);
    color: var(--text-faint);
    text-align: center;
    line-height: 1.4;
  }
  .note {
    margin-top: 16px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
</style>
