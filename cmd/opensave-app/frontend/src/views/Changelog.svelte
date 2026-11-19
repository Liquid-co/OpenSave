<script>
  import { onMount } from 'svelte';
  import Skeleton from '../components/ui/Skeleton.svelte';
  import { native } from '../lib/api.js';
  import { t } from '../lib/i18n.js';
  import ReleaseNotes from '../components/ReleaseNotes.svelte';
  import DiscordBanner from '../components/DiscordBanner.svelte';

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
</style>
