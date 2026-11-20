<script>
  // The top of Backups: where backups go, and how many games have the save
  // they are at now in the cloud, over a slanted wall of your own covers.
  // "Safe" means exactly that: the snapshot this PC's save is at is up there
  // (lib/backupsview.js, gameState).
  import { createEventDispatcher, onDestroy } from 'svelte';
  import ArrowUp from 'lucide-svelte/icons/arrow-up';
  import RotateCw from 'lucide-svelte/icons/rotate-cw';
  import Power from 'lucide-svelte/icons/power';
  import { providerById, isEmail, isOAuth } from '../../lib/cloudproviders.js';
  import { backupSummary } from '../../lib/backupsview.js';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { gameCover } from '../../lib/api.js';
  import { t } from '../../lib/i18n.js';
  import ProviderIcon from '../cloud/ProviderIcon.svelte';
  import CoverImage from '../../components/CoverImage.svelte';

  export let config;
  export let games = [];
  /** The tracked games, for the wall of covers. */
  export let tracked = [];
  export let on = true;
  export let reading = false;
  export let readError = '';
  /** { name, done, total } while uploads run. */
  export let uploading = null;
  /** The cloud has been read at least once. */
  export let loaded = false;
  /** False for a web address, which takes copies but cannot list them. */
  export let listable = true;

  const dispatch = createEventDispatcher();

  $: provider = providerById(config.provider);
  $: where = isOAuth(config.provider)
    ? isEmail(config.tokens?.userEmail)
      ? config.tokens.userEmail
      : ''
    : config.url;
  $: sum = backupSummary(games);

  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 60_000);
  onDestroy(() => clearInterval(tick));
  const ago = (ms) => {
    const p = timeAgoParts(new Date(ms).toISOString(), now);
    if (p.kind === 'invalid') return p.text;
    const { kind, ...params } = p;
    return $t('timeAgo.' + kind, params);
  };

  $: headline = !on
    ? $t('backups.hero.off')
    : !listable
      ? $t('backups.hero.webhook')
      : readError
      ? $t('backups.hero.cantReach', { provider: $t('cloud.provider.' + config.provider) })
      : !loaded
        ? $t('backups.hero.reading')
        : sum.tracked === 0
          ? $t('backups.hero.noneTracked')
          : sum.safe === sum.tracked
            ? $t('backups.hero.allSafe', { n: sum.tracked })
            : $t('backups.hero.someSafe', { safe: sum.safe, n: sum.tracked });

  $: sub = !on
    ? $t('backups.hero.offSub')
    : !listable
      ? $t('backups.hero.webhookSub')
      : readError
      ? readError
      : !loaded
        ? ''
        : [
            sum.lastMs ? $t('backups.hero.last', { ago: ago(sum.lastMs) }) : $t('backups.hero.nothingYet'),
            sum.waiting.length === 1
              ? $t('backups.hero.oneWaiting', { game: sum.waiting[0] })
              : sum.waiting.length > 1
                ? $t('backups.hero.manyWaiting', { n: sum.waiting.length })
                : ''
          ]
            .filter(Boolean)
            .join(' ');

  // The wall: fifteen tiles, your games over and over if there are fewer.
  $: wall = tracked.length ? Array.from({ length: 15 }, (_, i) => tracked[(i * 4 + 1) % tracked.length]) : [];
</script>

<section class="hero" aria-labelledby="backups-hero">
  {#if wall.length}
    <div class="wall" aria-hidden="true">
      {#each wall as g, i (i)}
        <span class="tile"><CoverImage src={gameCover(g, true)} alt="" /></span>
      {/each}
    </div>
  {/if}
  <div class="body">
    <div class="where">
      <span class="logo" class:brand={!!provider?.img}><ProviderIcon {provider} size={provider?.img ? 20 : 18} /></span>
      <span class="pname">{$t('cloud.provider.' + config.provider)}</span>
      {#if where}<span class="addr" title={where}>{where}</span>{/if}
    </div>
    <div>
      <h1 id="backups-hero">{headline}</h1>
      {#if uploading}
        <p class="sub" aria-live="polite">{$t('backups.hero.uploading', { game: uploading.name, done: Math.min(uploading.done + 1, uploading.total), total: uploading.total })}</p>
      {:else if sub}
        <p class="sub">{sub}</p>
      {/if}
    </div>
    <div class="actions">
      {#if !on}
        <button class="btn primary" on:click={() => dispatch('turnOn')}><Power size={15} />{$t('backups.hero.turnOn')}</button>
      {:else if !listable}
        <!-- Nothing to compare against, so nothing to send on demand. -->
      {:else if readError}
        <button class="btn primary" disabled={reading} on:click={() => dispatch('retry')}><RotateCw size={15} />{$t('backups.hero.retry')}</button>
      {:else}
        <button class="btn primary" disabled={!!uploading || !loaded || sum.tracked === 0} on:click={() => dispatch('backup')}><ArrowUp size={15} />{uploading ? $t('backups.hero.backingUp') : $t('backups.hero.backUpNow')}</button>
      {/if}
      <button class="btn glass" on:click={() => dispatch('change')}>{$t('backups.hero.changeWhere')}</button>
    </div>
  </div>
</section>

<style>
  .hero {
    position: relative;
    min-height: 220px;
    border-radius: var(--radius-xl);
    overflow: hidden;
    border: 1px solid var(--border);
    background: var(--bg-raised);
  }
  /* Your covers, slanted, fading out towards the words. A mask rather than
     a colour over them, so it fades into whatever the theme's panel is. */
  .wall {
    position: absolute;
    right: -40px;
    top: -70px;
    width: 560px;
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    gap: 12px;
    transform: rotate(-9deg);
    -webkit-mask-image: linear-gradient(90deg, transparent 0%, rgba(0, 0, 0, 0.35) 30%, rgba(0, 0, 0, 0.85) 70%, #000 100%);
    mask-image: linear-gradient(90deg, transparent 0%, rgba(0, 0, 0, 0.35) 30%, rgba(0, 0, 0, 0.85) 70%, #000 100%);
    animation: drift 40s linear infinite alternate;
  }
  @keyframes drift {
    to {
      transform: rotate(-9deg) translateY(-26px);
    }
  }
  .tile {
    position: relative;
    aspect-ratio: 2 / 3;
    border-radius: 8px;
    overflow: hidden;
    background: var(--bg-active);
    box-shadow: 0 10px 24px rgba(0, 0, 0, 0.45);
  }
  .tile :global(img) {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .body {
    position: relative;
    min-height: 220px;
    padding: 22px 26px;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    gap: 16px;
    max-width: 560px;
  }
  .where {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .logo {
    width: 32px;
    height: 32px;
    flex: none;
    border-radius: 9px;
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--text-dim);
  }
  .logo.brand {
    background: #fff;
  }
  .pname {
    font-weight: 600;
    white-space: nowrap;
  }
  .addr {
    min-width: 0;
    font-size: var(--text-md);
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  h1 {
    font-size: var(--text-2xl);
    font-weight: 650;
    letter-spacing: -0.01em;
    line-height: 1.15;
    text-wrap: balance;
  }
  .sub {
    margin-top: 8px;
    color: var(--text-dim);
    max-width: 480px;
  }
  .actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  .glass {
    background: rgba(255, 255, 255, 0.08);
    border-color: rgba(255, 255, 255, 0.14);
  }
  :global([data-theme='light']) .glass {
    background: rgba(20, 20, 40, 0.05);
    border-color: rgba(20, 20, 40, 0.12);
  }
</style>
