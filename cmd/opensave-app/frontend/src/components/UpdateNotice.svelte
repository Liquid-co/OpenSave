<script>
  // A newer OpenSave, at the top of the page with the other notices
  // (lib/update.js updateNotice): one waiting, with What's new and the button
  // that gets it; one going in, with how far along; or the word after one is
  // in. What's new opens the What's new page, which shows the release itself.
  import { fly as flyIn } from 'svelte/transition';
  import { gated } from '../lib/motion.js';
  import { navigate } from '../lib/stores.js';
  import { updateNotice, updateDismissed, justUpdated, installStarted, getUpdate, updateAction } from '../lib/update.js';
  import { t, segments } from '../lib/i18n.js';
  import ProgressBar from './ui/ProgressBar.svelte';
  import Sparkles from 'lucide-svelte/icons/sparkles';
  import Download from 'lucide-svelte/icons/download';
  import Wrench from 'lucide-svelte/icons/wrench';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import X from 'lucide-svelte/icons/x';

  const fly = gated(flyIn);
  $: n = $updateNotice;
  $: how = n?.kind === 'available' ? updateAction(n.update) : null;
  const bold = (key, params, names) => segments($t(key, params), params, names);
</script>

{#if n}
  <div class="notice" class:busy={n.kind === 'installing'} transition:fly={{ y: -20, duration: 200 }} role="status">
    <div class="icon">
      {#if n.kind !== 'installing'}<Sparkles size={19} />
      {:else if n.state === 'downloading'}<Download size={19} />
      {:else if n.state === 'installing'}<Wrench size={19} />
      {:else}<RefreshCw size={19} class="spin" />{/if}
    </div>
    <div class="body">
      {#if n.kind === 'available'}
        <div class="title">{#each bold('app.update.availableTitle', { latest: n.update.latest }, ['latest']) as p}{#if p.bold}<strong>{p.text}</strong>{:else}{p.text}{/if}{/each}</div>
        <div class="sub">{$t('app.update.availableSub', { current: n.update.current })}</div>
      {:else if n.kind === 'updated'}
        <div class="title">{#each bold('app.update.updatedTitle', { version: n.to }, ['version']) as p}{#if p.bold}<strong>{p.text}</strong>{:else}{p.text}{/if}{/each}</div>
        <div class="sub">{n.from ? $t('app.update.updatedSub', { from: n.from }) : $t('app.update.updatedSubPlain')}</div>
      {:else}
        <div class="title">
          {#if n.state === 'downloading'}{$t('app.updatingDownloading', { n: n.percentage })}
          {:else if n.state === 'installing'}{$t('app.installingUpdate')}
          {:else}{$t('app.restartingVersion')}{/if}
        </div>
        <div class="sub">{$t('app.restartsItself')}</div>
        {#if n.state === 'downloading'}<div class="bar"><ProgressBar done={n.percentage} total={100} /></div>{/if}
      {/if}
    </div>
    {#if n.kind !== 'installing'}
      <div class="actions">
        <button class="btn small" on:click={() => navigate('changelog')}>{$t('app.whatsNew')}</button>
        {#if n.kind === 'available'}
          <button class="btn small primary" disabled={how === 'install' && $installStarted} on:click={getUpdate}>
            {#if how === 'install'}{$installStarted ? $t('app.starting') : $t('app.installRestart')}
            {:else if how === 'flatpak'}{$t('app.getFlatpak')}
            {:else}{$t('app.download')}{/if}
          </button>
        {/if}
        <button
          class="close"
          aria-label={$t('common.dismiss')}
          title={$t('common.dismiss')}
          on:click={() => (n.kind === 'available' ? updateDismissed.set(true) : justUpdated.set(null))}><X size={15} /></button
        >
      </div>
    {/if}
  </div>
{/if}

<style>
  /* The same card as the other notices (NewGamesBanner): raised, in the
     accent's outline, an icon in a tinted square, the words, the buttons. */
  .notice {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px 14px;
    padding: 10px 10px 10px 14px;
    background: var(--bg-raised);
    border: 1px solid var(--accent);
    border-radius: var(--radius-lg);
    /* Part of the page, not floating over it: a light shadow, enough to
       lift it off the cover art behind. */
    box-shadow:
      0 4px 14px rgba(0, 0, 0, 0.28),
      0 0 0 1px var(--accent-soft);
  }
  .notice.busy {
    border-color: var(--border-strong);
    box-shadow: none;
  }
  .icon {
    flex-shrink: 0;
    width: 36px;
    height: 36px;
    border-radius: calc(10px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .icon :global(.spin) {
    animation: spin 1.2s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .body {
    flex: 1 1 320px;
    min-width: 0;
  }
  .title {
    font-size: 0.92rem;
  }
  .sub {
    margin-top: 2px;
    font-size: 0.78rem;
    color: var(--text-faint);
  }
  .bar {
    margin-top: 8px;
    max-width: 360px;
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-left: auto;
    flex-shrink: 0;
  }
  .close {
    width: 30px;
    height: 30px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: calc(8px * var(--round, 1));
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  .close:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
</style>
