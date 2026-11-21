<script>
  // The themes, as cards: each a small picture of the app in it — its
  // sidebar, your own games' covers, a panel and a button in its accent — so
  // what is picked can be seen before it is. The one in use is ringed.
  // Picking one changes the app at once (lib/themes.js pick).
  //
  // After the app's own come yours, each with a menu — change it, copy it,
  // save it as a file, delete it (with Undo) — and last a card to make one.
  import { createEventDispatcher } from 'svelte';
  import { appearance, sanitizeAppearance, customFamily, THEME_ACCENT } from '../../lib/appearance.js';
  import { LOOKS, FAMILIES, lookFor, pick } from '../../lib/themes.js';
  import { deriveLook, uniqueName, newId } from '../../lib/customthemes.js';
  import { exportToFile } from '../../lib/themefiles.js';
  import { gameList, toast } from '../../lib/stores.js';
  import { gameCover } from '../../lib/api.js';
  import { pop } from '../../lib/motion.js';
  import CoverImage from '../../components/CoverImage.svelte';
  import Dropdown from '../../components/ui/Dropdown.svelte';
  import Check from 'lucide-svelte/icons/check';
  import Plus from 'lucide-svelte/icons/plus';
  import MoreHorizontal from 'lucide-svelte/icons/ellipsis';
  import Pencil from 'lucide-svelte/icons/pencil';
  import Copy from 'lucide-svelte/icons/copy';
  import Download from 'lucide-svelte/icons/download';
  import Trash2 from 'lucide-svelte/icons/trash-2';
  import { t } from '../../lib/i18n.js';

  const dispatch = createEventDispatcher();
  const prefersDark = () => globalThis.matchMedia?.('(prefers-color-scheme: dark)').matches ?? true;

  $: a = sanitizeAppearance($appearance);
  const sub = (family) => {
    if (family === 'opensave') return $t('theme.sub.default');
    const f = FAMILIES[family];
    if (f.dark && f.light) return $t('theme.sub.both');
    return $t(f.dark ? 'theme.sub.dark' : 'theme.sub.light');
  };
  // Every card: the app's own — showing the look they would now, dark or
  // light as chosen — then yours.
  $: cards = [
    ...Object.keys(FAMILIES).map((family) => ({ key: family, family, look: LOOKS[lookFor(family, a.theme, prefersDark())], name: $t('theme.name.' + family), sub: sub(family), mine: null })),
    ...a.customThemes.map((m) => ({ key: m.id, family: customFamily(m.id), look: deriveLook(m), name: m.name, sub: $t('theme.sub.yours'), mine: m }))
  ];
  // Your games, for the covers in every picture; the first four with art.
  $: covers = $gameList.slice(0, 4);

  // ── Your own ──
  let menuFor = null;
  $: builtInNames = Object.keys(FAMILIES).map((f) => $t('theme.name.' + f));
  function duplicate(m) {
    const list = sanitizeAppearance($appearance).customThemes;
    const copy = { ...structuredClone(m), id: newId(), name: uniqueName($t('themeEditor.copyName', { name: m.name }), list.map((x) => x.name), builtInNames) };
    appearance.update((v) => ({ ...v, customThemes: [...list, copy] }));
  }
  function remove(m) {
    const before = sanitizeAppearance($appearance);
    const wasOn = before.family === customFamily(m.id);
    appearance.update((v) => ({
      ...v,
      customThemes: before.customThemes.filter((x) => x.id !== m.id),
      ...(wasOn ? { family: 'opensave', accent: THEME_ACCENT } : {})
    }));
    toast($t('themeEditor.deleted', { name: m.name }), 'info', {
      undo: true,
      ttl: 8000,
      action: {
        label: $t('toast.undo'),
        run: () => appearance.update((v) => ({ ...v, customThemes: before.customThemes, ...(wasOn ? { family: before.family, accent: before.accent } : {}) }))
      }
    });
  }
  const menuItems = (m) => [
    { label: $t('themeEditor.menu.edit'), icon: Pencil, run: () => dispatch('edit', m) },
    { label: $t('themeEditor.menu.duplicate'), icon: Copy, run: () => duplicate(m) },
    { label: $t('themeEditor.menu.export'), icon: Download, run: () => exportToFile(m, $t) },
    null,
    { label: $t('themeEditor.menu.delete'), icon: Trash2, danger: true, run: () => remove(m) }
  ];
</script>

<div class="cards" role="radiogroup" aria-label={$t('settings.appearance.theme')}>
  {#each cards as c (c.key)}
    {@const look = c.look}
    {@const on = a.family === c.family}
    <div class="card" class:on in:pop={{ start: 0.94 }}>
      <button
        class="pick"
        role="radio"
        aria-checked={on}
        style="--l-bg: {look.bg}; --l-side: {look.sidebar}; --l-raised: {look.raised}; --l-active: {look.active}; --l-border: {look.border}; --l-text: {look.text}; --l-faint: {look.textFaint}; --l-accent: {look.accent}"
        on:click={() => appearance.update((v) => pick(c.family, v))}
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
          <span class="name">{c.name}</span>
          <span class="sub" class:yours={!!c.mine}>{c.sub}</span>
        </span>
      </button>
      {#if c.mine}
        <span class="menu-at">
          <button class="menu-btn" class:open={menuFor === c.key} aria-label={$t('themeEditor.menu.more', { name: c.name })} aria-haspopup="menu" aria-expanded={menuFor === c.key} on:click={() => (menuFor = menuFor === c.key ? null : c.key)}
            ><MoreHorizontal size={15} /></button
          >
          <Dropdown items={menuItems(c.mine)} open={menuFor === c.key} align="left" label={c.name} on:close={() => (menuFor = null)} />
        </span>
      {/if}
    </div>
  {/each}
  <button class="create" on:click={() => dispatch('create')}><span class="plus"><Plus size={18} /></span>{$t('themeEditor.createCard')}</button>
</div>

<style>
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
    gap: 12px;
  }
  .card {
    position: relative;
    border-radius: calc(12px * var(--round, 1));
    background: var(--bg);
    box-shadow: inset 0 0 0 1px var(--border);
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
  .pick {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: 100%;
    padding: 8px 8px 10px;
    border: 0;
    border-radius: inherit;
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  /* Your own: a menu, in the picture's top corner, there while pointed at. */
  .menu-at {
    position: absolute;
    top: 13px;
    left: 13px;
  }
  .menu-btn {
    width: 26px;
    height: 26px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: calc(7px * var(--round, 1));
    background: rgba(0, 0, 0, 0.55);
    color: #fff;
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .card:hover .menu-btn,
  .menu-btn:focus-visible,
  .menu-btn.open {
    opacity: 1;
  }
  .menu-btn :global(svg) {
    display: block;
  }
  .sub.yours {
    color: var(--accent-text);
  }
  /* Making one: a card of its own, last. */
  .create {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    min-height: 132px;
    border: 1.5px dashed var(--border-strong);
    border-radius: calc(12px * var(--round, 1));
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-md);
    font-weight: 600;
    cursor: pointer;
    transition:
      border-color 0.15s,
      color 0.15s,
      background 0.15s;
  }
  .create:hover {
    border-color: var(--accent);
    color: var(--text);
    background: rgba(var(--accent-rgb), 0.06);
  }
  .plus {
    width: 34px;
    height: 34px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    background: var(--bg-active);
    color: var(--accent-text);
    transition: transform 0.2s cubic-bezier(0.3, 1.5, 0.5, 1);
  }
  .create:hover .plus {
    transform: rotate(90deg);
  }

  /* The picture: the app, small, in that theme's colours. */
  .pic {
    position: relative;
    display: flex;
    height: 86px;
    overflow: hidden;
    border-radius: calc(8px * var(--round, 1));
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
    border-radius: calc(3px * var(--round, 1));
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
    border-radius: calc(3px * var(--round, 1));
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
    border-radius: calc(3px * var(--round, 1));
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
    border-radius: calc(3px * var(--round, 1));
    background: var(--l-raised);
    box-shadow: inset 0 0 0 1px var(--l-border);
  }
  .btn-mock {
    width: 24px;
    height: 11px;
    border-radius: calc(3px * var(--round, 1));
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
