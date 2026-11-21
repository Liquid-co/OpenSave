<script>
  // The themes, as cards: each a small picture of the app in it — its
  // sidebar, your own games' covers, a panel and a button in its accent — so
  // what is picked can be seen before it is. The one in use is ringed.
  // Picking one changes the app at once (lib/themes.js pick).
  import { appearance, sanitizeAppearance } from '../../lib/appearance.js';
  import { THEMES_BUILT_IN, LOOKS, FAMILIES, lookFor, isCurrent, pick } from '../../lib/themes.js';
  import { gameList } from '../../lib/stores.js';
  import { gameCover } from '../../lib/api.js';
  import CoverImage from '../../components/CoverImage.svelte';
  import Check from 'lucide-svelte/icons/check';
  import { t } from '../../lib/i18n.js';

  const prefersDark = () => globalThis.matchMedia?.('(prefers-color-scheme: dark)').matches ?? true;

  $: a = sanitizeAppearance($appearance);
  $: lookNow = lookFor(a.family, a.theme, prefersDark());
  // The look each card shows: its own, or — for a family it stands for whole
  // (Catppuccin) — the one it would show now.
  const lookOf = (theme, a) => theme.look ?? lookFor(theme.family, a.theme, prefersDark());
  // Your games, for the covers in every picture; the first four with art.
  $: covers = $gameList.slice(0, 4);
  const sub = (theme) => {
    if (theme.id === 'night') return $t('theme.sub.default');
    const f = FAMILIES[theme.family];
    if (!theme.look && f.dark && f.light) return $t('theme.sub.both');
    return $t(LOOKS[theme.look ?? f.dark].kind === 'dark' ? 'theme.sub.dark' : 'theme.sub.light');
  };
</script>

<div class="cards" role="radiogroup" aria-label={$t('settings.appearance.theme')}>
  {#each THEMES_BUILT_IN as theme (theme.id)}
    {@const look = LOOKS[lookOf(theme, a)]}
    {@const on = isCurrent(theme, a.family, lookNow)}
    <button
      class="card"
      class:on
      role="radio"
      aria-checked={on}
      style="--l-bg: {look.bg}; --l-side: {look.sidebar}; --l-raised: {look.raised}; --l-active: {look.active}; --l-border: {look.border}; --l-text: {look.text}; --l-faint: {look.textFaint}; --l-accent: {look.accent}"
      on:click={() => appearance.update((v) => pick(theme, v))}
    >
      <span class="pic" aria-hidden="true">
        <span class="side">
          <span class="bar accent"></span>
          <span class="bar"></span>
          <span class="bar short"></span>
        </span>
        <span class="main">
          <span class="title"></span>
          <span class="covers">
            {#each [0, 1, 2, 3] as i}
              <span class="cv">
                {#if covers[i]}<CoverImage src={gameCover(covers[i], true)} alt="" />{/if}
              </span>
            {/each}
          </span>
          <span class="foot"><span class="panel"></span><span class="btn-mock"></span></span>
        </span>
        {#if on}<span class="tick"><Check size={12} strokeWidth={3} /></span>{/if}
      </span>
      <span class="words">
        <span class="name">{$t('theme.name.' + lookOf(theme, a))}</span>
        <span class="sub">{sub(theme)}</span>
      </span>
    </button>
  {/each}
</div>

<style>
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
    gap: 12px;
  }
  .card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 8px 8px 10px;
    border: 0;
    border-radius: 12px;
    background: var(--bg);
    box-shadow: inset 0 0 0 1px var(--border);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      box-shadow 0.18s,
      transform 0.18s cubic-bezier(0.2, 0.7, 0.2, 1.3);
  }
  .card:hover {
    transform: translateY(-2px);
    box-shadow: inset 0 0 0 1px var(--border-strong);
  }
  .card:active {
    transform: scale(0.98);
  }
  .card.on {
    box-shadow: 0 0 0 2px var(--accent);
  }

  /* The picture: the app, small, in that theme's colours. */
  .pic {
    position: relative;
    display: flex;
    height: 86px;
    overflow: hidden;
    border-radius: 8px;
    background: var(--l-bg);
    box-shadow: inset 0 0 0 1px rgba(127, 127, 127, 0.18);
    transition: background 0.3s;
  }
  .side {
    width: 24%;
    display: flex;
    flex-direction: column;
    gap: 5px;
    padding: 9px 6px;
    background: var(--l-side);
    border-right: 1px solid var(--l-border);
  }
  .bar {
    height: 5px;
    width: 70%;
    border-radius: 3px;
    background: var(--l-active);
  }
  .bar.accent {
    width: 85%;
    background: var(--l-accent);
  }
  .bar.short {
    width: 50%;
  }
  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px;
  }
  .title {
    height: 6px;
    width: 45%;
    border-radius: 3px;
    background: var(--l-text);
    opacity: 0.85;
  }
  .covers {
    flex: 1;
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 4px;
  }
  .cv {
    position: relative;
    overflow: hidden;
    border-radius: 3px;
    background: var(--l-active);
  }
  .cv :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .foot {
    display: flex;
    gap: 5px;
  }
  .panel {
    flex: 1;
    height: 11px;
    border-radius: 3px;
    background: var(--l-raised);
    box-shadow: inset 0 0 0 1px var(--l-border);
  }
  .btn-mock {
    width: 24px;
    height: 11px;
    border-radius: 3px;
    background: var(--l-accent);
  }
  .tick {
    position: absolute;
    top: 6px;
    right: 6px;
    width: 20px;
    height: 20px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    background: var(--accent-fill);
    color: var(--on-accent);
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.35);
    animation: tick-in 0.3s cubic-bezier(0.3, 1.6, 0.5, 1);
  }
  .tick :global(svg) {
    display: block;
  }
  @keyframes tick-in {
    from {
      transform: scale(0.3);
    }
  }
  .words {
    display: flex;
    align-items: baseline;
    gap: 6px;
    padding: 0 4px;
  }
  .name {
    flex: 1;
    min-width: 0;
    font-size: var(--text-md);
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .sub {
    flex: none;
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
</style>
