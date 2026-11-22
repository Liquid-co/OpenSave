<script>
  // Everything that has changed, newest first — and, when a newer OpenSave is
  // waiting, that one at the top, with what it changes and the button that
  // gets it (lib/update.js, the same as the notice at the top of the page).
  import { onMount } from 'svelte';
  import Skeleton from '../components/ui/Skeleton.svelte';
  import { native } from '../lib/api.js';
  import { t } from '../lib/i18n.js';
  import { availableUpdate } from '../lib/stores.js';
  import { justUpdated, installStarted, getUpdate, updateAction } from '../lib/update.js';
  import ReleaseNotes from '../components/ReleaseNotes.svelte';
  import DiscordBanner from '../components/DiscordBanner.svelte';
  import Sparkles from 'lucide-svelte/icons/sparkles';

  // Here is where "OpenSave was updated" points; once here, it has been seen.
  justUpdated.set(null);

  $: next = $availableUpdate?.available ? $availableUpdate : null;
  $: how = updateAction(next);

  let releases = [];
  let current = '';
  let loading = true;

  onMount(async () => {
    try {
      const [rel, info] = await Promise.all([native.changelogReleases(), native.appInfo()]);
      releases = rel ?? [];
      current = info?.version ?? '';
    } catch {
      releases = [];
    }
    loading = false;
  });
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>{$t('about.changelog')}</h1>
      <p class="sub">{$t('changelog.sub')}</p>
    </div>
    {#if current}
      <div class="running" title={$t('changelog.runningTitle')}>
        <span>{$t('changelog.running')}</span>
        {current}
      </div>
    {/if}
  </header>

  {#if next}
    <section class="next" aria-labelledby="next-title">
      <div class="next-head">
        <span class="next-icon"><Sparkles size={18} /></span>
        <div class="next-words">
          <span class="kicker">{$t('changelog.next.kicker')}</span>
          <h2 id="next-title">{$t('changelog.next.title', { version: next.latest })}</h2>
        </div>
        <button class="btn primary" disabled={how === 'install' && $installStarted} on:click={getUpdate}>
          {#if how === 'install'}{$installStarted ? $t('app.starting') : $t('app.installRestart')}
          {:else if how === 'flatpak'}{$t('app.getFlatpak')}
          {:else}{$t('app.download')}{/if}
        </button>
      </div>
      {#if next.release}
        <div class="next-notes"><ReleaseNotes releases={[next.release]} compact /></div>
      {:else}
        <p class="next-none">
          {$t('changelog.next.noNotes')}
          <button class="gh-link" on:click={() => native.openExternal(next.url)}>{$t('changelog.next.onGitHub')}</button>
        </p>
      {/if}
    </section>
  {/if}

  <div class="banner-slot"><DiscordBanner /></div>

  {#if loading}
    <Skeleton kind="lines" count={8} />
  {:else}
    <ReleaseNotes {releases} />
  {/if}
</div>

<style>
  /* A column to read, held to a readable line length and centred in the
     window. Left-aligned, a maximised window left it in one corner with most
     of the screen empty beside it. */
  .page {
    padding: 26px 30px 40px;
    max-width: 820px;
    margin: 0 auto;
  }
  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 26px;
  }
  h1 {
    font-size: 1.5rem;
    font-weight: 700;
    letter-spacing: -0.02em;
  }
  .sub {
    color: var(--text-dim);
    font-size: 0.88rem;
    margin-top: 4px;
  }
  .running {
    flex-shrink: 0;
    text-align: right;
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--accent-text);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 8px 12px;
    background: var(--bg-raised);
  }
  .running span {
    display: block;
    font-size: 0.66rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--text-faint);
    margin-bottom: 1px;
  }
  .banner-slot {
    margin-bottom: 26px;
  }

  /* The release waiting to be installed: a card in the accent's outline,
     like the notice that points here, above everything already installed. */
  .next {
    margin-bottom: 26px;
    padding: 18px 20px 20px;
    background: var(--bg-raised);
    border: 1px solid var(--accent);
    border-radius: var(--radius-lg);
    box-shadow: 0 0 0 1px var(--accent-soft);
  }
  .next-head {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .next-icon {
    flex-shrink: 0;
    width: 38px;
    height: 38px;
    display: grid;
    place-items: center;
    border-radius: calc(10px * var(--round, 1));
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .next-words {
    flex: 1;
    min-width: 0;
  }
  .kicker {
    display: block;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--accent-text);
  }
  h2 {
    font-size: 1.15rem;
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .next-notes {
    margin-top: 16px;
    padding-top: 16px;
    border-top: 1px solid var(--border);
  }
  .next-none {
    margin-top: 12px;
    font-size: 0.86rem;
    color: var(--text-dim);
  }
  .gh-link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent-text);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }
  .gh-link:hover {
    text-decoration: underline;
  }
</style>
