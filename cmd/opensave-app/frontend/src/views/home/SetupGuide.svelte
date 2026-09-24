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

  /** Whether a scan is running, from the page's scan dialog. */
  export let scanning = false;

  const dispatch = createEventDispatcher();

  $: plan = setupSteps({
    games: $gameList.length,
    peers: Object.keys($peers).length,
    cloud: $settings?.cloudSync?.enabled,
    skipped: $setupState.skipped
  });
  const skip = (id) => setupState.update((s) => ({ ...s, skipped: [...new Set([...s.skipped, id])] }));
  const later = () => setupState.update((s) => ({ ...s, dismissed: true }));

  const copy = {
    saves: {
      icon: ScanSearch,
      title: 'Find your game saves',
      text: 'OpenSave looks through Steam, emulators and the usual save folders, and you choose what to keep.'
    },
    devices: {
      icon: MonitorSmartphone,
      title: 'Add your other devices',
      text: 'A PC, a Steam Deck or a handheld with OpenSave on it — on your Wi-Fi, or anywhere over the internet.'
    },
    cloud: {
      icon: Cloud,
      title: 'Back up to the cloud',
      text: 'A copy of every snapshot in Google Drive, OneDrive, Dropbox or a folder of your own.'
    }
  };
</script>

<div class="card guide">
  <div class="top">
    <div>
      <h3>Get OpenSave set up</h3>
      <p class="progress">{plan.settled} of 3 done</p>
    </div>
    <button class="btn small ghost" on:click={later} title="Hide this guide"><X size={14} />Later</button>
  </div>
  <ol>
    {#each plan.steps as step, i}
      {@const c = copy[step.id]}
      <li class:current={plan.next === step.id} class:done={step.done} class:skipped={step.skipped}>
        <span class="mark">
          {#if step.done}<Check size={14} strokeWidth={3} />{:else}{i + 1}{/if}
        </span>
        <div class="body">
          <div class="title">{c.title}{#if step.skipped}<span class="tag">Skipped</span>{/if}</div>
          {#if plan.next === step.id}
            <p class="text">{c.text}</p>
            <div class="actions">
              {#if step.id === 'saves'}
                <button class="btn small primary" disabled={scanning} on:click={() => dispatch('scan')}>
                  <ScanSearch size={14} />{scanning ? 'Scanning…' : 'Scan for saves'}
                </button>
                <button class="btn small" on:click={() => dispatch('add')}><FolderPlus size={14} />Pick a folder</button>
              {:else if step.id === 'devices'}
                <button class="btn small primary" on:click={() => navigate('devices')}><MonitorSmartphone size={14} />Pair a device</button>
                <button class="btn small ghost" on:click={() => skip('devices')}>Only this device for now</button>
              {:else}
                <button class="btn small primary" on:click={() => navigate('cloud')}><Cloud size={14} />Choose where</button>
                <button class="btn small ghost" on:click={() => skip('cloud')}>No cloud backup</button>
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
    color: var(--accent);
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
