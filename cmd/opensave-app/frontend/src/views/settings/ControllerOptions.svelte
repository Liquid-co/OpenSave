<script>
  // Using OpenSave with a gamepad, on this device (lib/controller.js). Kept
  // with the appearance in local storage: it is about this screen and what
  // is in the hands in front of it.
  import { slidingIndicator } from '../../lib/motion.js';
  import { appearance } from '../../lib/appearance.js';
  import { CONTROLLER_MODES, controllerOn, padUsed } from '../../lib/controller.js';
  import { settings } from '../../lib/stores.js';
  import { t } from '../../lib/i18n.js';

  const set = (patch) => appearance.update((v) => ({ ...v, ...patch }));

  // What each button does, in the order someone reaches for them.
  $: BUTTONS = [
    { keys: ['D-pad', 'Left stick'], does: $t('settings.controller.move') },
    { keys: ['A'], does: $t('settings.controller.press') },
    { keys: ['B'], does: $t('settings.controller.close') },
    { keys: ['X'], does: $t('settings.controller.menu') },
    { keys: ['Y', 'Start'], does: $t('settings.controller.quickSwitcher') },
    { keys: ['LB', 'RB'], does: $t('settings.controller.page') }
  ];

  $: active = controllerOn($appearance.controller, { deviceType: $settings?.deviceType, used: $padUsed });
</script>

<div class="row">
  <div class="segmented" role="radiogroup" aria-label={$t('settings.general.controller')} use:slidingIndicator={{ mode: 'fill' }}>
    {#each Object.entries(CONTROLLER_MODES) as [id, label]}
      <button role="radio" aria-checked={$appearance.controller === id} class:on={$appearance.controller === id} on:click={() => set({ controller: id })}>
        {$t('controller.mode.' + id)}
      </button>
    {/each}
  </div>
  <span class="state" class:is-on={active}>
    {#if $appearance.controller === 'on'}{$t('settings.controller.on')}
    {:else if $appearance.controller === 'off'}{$t('settings.controller.off')}
    {:else if active}{$t('settings.controller.onNow')}
    {:else}{$t('settings.controller.auto')}{/if}
  </span>
</div>

<dl class="buttons" aria-label={$t('settings.controller.buttonsAria')}>
  {#each BUTTONS as b}
    <dt>
      {#each b.keys as k, i}{#if i > 0}<span class="or">{$t('settings.controller.or')}</span>{/if}<kbd>{k}</kbd>{/each}
    </dt>
    <dd>{b.does}</dd>
  {/each}
</dl>
<p class="hint">{$t('settings.controller.hint')}</p>

<style>
  .row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px 16px;
  }
  .segmented {
    display: inline-flex;
    border: 1px solid var(--border-strong);
    border-radius: 9px;
    overflow: hidden;
  }
  .segmented button {
    padding: 7px 14px;
    background: var(--bg);
    border: none;
    border-left: 1px solid var(--border-strong);
    color: var(--text-dim);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
  }
  .segmented button:first-child {
    border-left: none;
  }
  .segmented button.on {
    background: var(--accent-soft);
    color: var(--text);
    font-weight: 600;
  }
  .state {
    font-size: 0.82rem;
    color: var(--text-faint);
  }
  .state.is-on {
    color: var(--text-dim);
  }
  .buttons {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 7px 16px;
    align-items: center;
    margin: 16px 0 0;
    font-size: 0.84rem;
  }
  dt {
    display: flex;
    align-items: center;
    gap: 5px;
  }
  dd {
    margin: 0;
    color: var(--text-dim);
  }
  kbd {
    display: inline-block;
    min-width: 26px;
    padding: 2px 7px;
    border: 1px solid var(--border-strong);
    border-bottom-width: 2px;
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-size: 0.78rem;
    font-weight: 600;
    text-align: center;
  }
  .or {
    font-size: 0.75rem;
    color: var(--text-faint);
  }
  .hint {
    margin: 12px 0 0;
  }
</style>
