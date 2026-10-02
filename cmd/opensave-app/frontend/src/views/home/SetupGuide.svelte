<script>
  // Three steps to a working setup, each ticked off by the thing itself
  // existing. See lib/setup.js.
  import { createEventDispatcher } from 'svelte';
  import Check from 'lucide-svelte/icons/check';
  import ScanSearch from 'lucide-svelte/icons/scan-search';
  import FolderPlus from 'lucide-svelte/icons/folder-plus';
  import MonitorSmartphone from 'lucide-svelte/icons/monitor-smartphone';
  import Cloud from 'lucide-svelte/icons/cloud';
  import X from 'lucide-svelte/icons/x';
  import { gameList, peers, settings, navigate } from '../../lib/stores.js';
  import { setupState, setupSteps } from '../../lib/setup.js';
  import { t } from '../../lib/i18n.js';

  /** Whether a scan is running, from the page's scan dialog. */
  export let scanning = false;

  const dispatch = createEventDispatcher();

  $: plan = setupSteps({
    games: $gameList.length,
    peers: Object.keys($peers).length,
    cloud: $settings?.cloudSync?.ready,
    skipped: $setupState.skipped
  });
  const skip = (id) => setupState.update((s) => ({ ...s, skipped: [...new Set([...s.skipped, id])] }));
  const later = () => setupState.update((s) => ({ ...s, dismissed: true }));

  // Steps carry i18n keys rather than text: the copy has to follow the picked
  // language, which means reading `t` at render time, not once here.
  const copy = {
    saves: {
      icon: ScanSearch,
      titleKey: 'home.setup.stepSaves.title',
      textKey: 'home.setup.stepSaves.text'
    },
    devices: {
      icon: MonitorSmartphone,
      titleKey: 'home.setup.stepDevices.title',
      textKey: 'home.setup.stepDevices.text'
    },
    cloud: {
      icon: Cloud,
      titleKey: 'home.setup.stepCloud.title',
      textKey: 'home.setup.stepCloud.text'
    }
  };
</script>

<div class="card guide">
  <div class="top">
    <div>
      <h3>{$t('home.setup.title')}</h3>
      <p class="progress">{$t('home.setup.progress', { n: plan.settled })}</p>
    </div>
    <button class="btn small ghost" on:click={later} title={$t('home.setup.hideTitle')}><X size={14} />{$t('home.setup.later')}</button>
  </div>
  <ol>
    {#each plan.steps as step, i}
      {@const c = copy[step.id]}
      <li class:current={plan.next === step.id} class:done={step.done} class:skipped={step.skipped}>
        <span class="mark">
          {#if step.done}<Check size={14} strokeWidth={3} />{:else}{i + 1}{/if}
        </span>
        <div class="body">
          <div class="title">{$t(c.titleKey)}{#if step.skipped}<span class="tag">{$t('home.setup.skipped')}</span>{/if}</div>
          {#if plan.next === step.id}
            <p class="text">{$t(c.textKey)}</p>
            <div class="actions">
              {#if step.id === 'saves'}
                <button class="btn small primary" disabled={scanning} on:click={() => dispatch('scan')}>
                  <ScanSearch size={14} />{scanning ? $t('home.scanning') : $t('home.welcome.scanForSaves')}
                </button>
                <button class="btn small" on:click={() => dispatch('add')}><FolderPlus size={14} />{$t('home.setup.pickFolder')}</button>
              {:else if step.id === 'devices'}
                <button class="btn small primary" on:click={() => navigate('devices')}><MonitorSmartphone size={14} />{$t('home.setup.pairDevice')}</button>
                <button class="btn small ghost" on:click={() => skip('devices')}>{$t('home.setup.onlyThisDevice')}</button>
              {:else}
                <button class="btn small primary" on:click={() => navigate('cloud')}><Cloud size={14} />{$t('home.setup.chooseWhere')}</button>
                <button class="btn small ghost" on:click={() => skip('cloud')}>{$t('home.setup.noCloudBackup')}</button>
              {/if}
            </div>
          {/if}
        </div>
      </li>
    {/each}
  </ol>
</div>

<style>
  .guide {
    margin-bottom: 18px;
    padding: 18px 20px;
    background:
      radial-gradient(120% 140% at 0% 0%, var(--accent-soft), transparent 60%),
      var(--bg-raised);
  }
  .top {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 10px;
  }
  h3 {
    font-size: 1.05rem;
  }
  .progress {
    margin-top: 2px;
    font-size: 0.8rem;
    color: var(--text-faint);
  }
  ol {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  li {
    display: flex;
    gap: 12px;
    padding: 8px 0;
  }
  .mark {
    flex-shrink: 0;
    width: 26px;
    height: 26px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    font-size: 0.78rem;
    font-weight: 700;
    border: 1.5px solid var(--border-strong);
    color: var(--text-faint);
  }
  li.current .mark {
    border-color: var(--accent);
    color: var(--accent-text);
  }
  li.done .mark {
    border-color: transparent;
    background: var(--success);
    color: #fff;
  }
  .body {
    flex: 1;
    min-width: 0;
    padding-top: 3px;
  }
  .title {
    font-weight: 600;
    font-size: 0.92rem;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  li.done .title,
  li.skipped .title {
    color: var(--text-dim);
    font-weight: 500;
  }
  .tag {
    font-size: 0.68rem;
    font-weight: 600;
    padding: 0 7px;
    border-radius: 999px;
    background: var(--btn-bg);
    color: var(--text-faint);
  }
  .text {
    margin: 4px 0 10px;
    font-size: 0.85rem;
    color: var(--text-dim);
    line-height: 1.5;
  }
  .actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
</style>
