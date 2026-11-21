<script>
  // Making a theme, or changing one made before (the ThemeEditorV3 board):
  // a name and a place to start from; six colours, and four more if wanted;
  // the shape of everything — corners, borders, shadows — and the font. A
  // picture of the app in it, changing as you go, and how readable its text
  // is, with Fix where it is not. Saved, it is put on at once.
  //
  // Nothing here touches the app until it is saved: the picture is the
  // theme, the app behind it is still yours.
  import { createEventDispatcher } from 'svelte';
  import { get } from 'svelte/store';
  import { appearance, sanitizeAppearance, customFamily, THEME_ACCENT } from '../../lib/appearance.js';
  import { LOOKS, FAMILIES, lookVars } from '../../lib/themes.js';
  import { sanitizeTheme, deriveLook, readability, fixCheck, themeFromLook, uniqueName, FONTS, SHAPE, COLOURS, MORE_COLOURS } from '../../lib/customthemes.js';
  import { exportToFile } from '../../lib/themefiles.js';
  import { normalizeHex } from '../../lib/colour.js';
  import { gameList, toast } from '../../lib/stores.js';
  import { gameCover } from '../../lib/api.js';
  import { collapse } from '../../lib/motion.js';
  import { t } from '../../lib/i18n.js';
  import Select from '../../components/ui/Select.svelte';
  import CoverImage from '../../components/CoverImage.svelte';
  import AccentWheel from './AccentWheel.svelte';
  import ArrowLeft from 'lucide-svelte/icons/arrow-left';
  import Check from 'lucide-svelte/icons/check';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import Download from 'lucide-svelte/icons/download';
  import ChevronDown from 'lucide-svelte/icons/chevron-down';
  import Plus from 'lucide-svelte/icons/plus';
  import RotateCcw from 'lucide-svelte/icons/rotate-ccw';

  /** The theme to change, or a new one to make. */
  export let theme;
  export let isNew = true;

  const dispatch = createEventDispatcher();
  let draft = structuredClone(sanitizeTheme(theme));

  $: look = deriveLook(draft);
  $: checks = readability(draft);
  $: failing = checks.filter((c) => !c.ok);
  // The picture's own colours, shape and type, set on it alone.
  $: previewStyle = [
    ...Object.entries(lookVars(look)),
    ['--accent', draft.colours.accent],
    ['--accent-fill', look.accentFill],
    ['--accent-text', draft.colours.accent],
    ['--on-accent', look.onAccent],
    ['--round', String(draft.roundness)],
    ['--pv-shadow', `0 ${10 * draft.shadows}px ${24 * draft.shadows}px rgba(0, 0, 0, ${(look.kind === 'dark' ? 0.5 : 0.16) * Math.min(draft.shadows, 1.4)})`],
    ['font-family', look.font]
  ]
    .map(([k, v]) => `${k}: ${v}`)
    .join('; ');

  // ── Where it starts from ──
  $: mine = sanitizeAppearance($appearance).customThemes;
  $: startOptions = [
    ...Object.entries(FAMILIES).flatMap(([family, f]) =>
      ['dark', 'light']
        .filter((k) => f[k])
        .map((k) => ({
          value: 'look:' + f[k],
          label: f.dark && f.light ? `${$t('theme.name.' + family)} · ${$t('theme.sub.' + k)}` : $t('theme.name.' + family)
        }))
    ),
    ...mine.filter((m) => m.id !== draft.id).map((m) => ({ value: 'mine:' + m.id, label: m.name }))
  ];
  let startFrom = '';
  function restart(value) {
    startFrom = value;
    const [kind, id] = value.split(':');
    const seed = kind === 'look' ? themeFromLook(LOOKS[id]) : sanitizeTheme(mine.find((m) => m.id === id));
    if (!seed) return;
    draft = { ...seed, id: draft.id, name: draft.name };
  }

  // ── Colours ──
  let wheelFor = null;
  // What a swatch shows: the colour picked, or the one worked out for it.
  // Reactive, so every swatch follows the theme as it changes.
  $: shown = (k) => draft.colours[k] || (k === 'onAccent' ? look.onAccent : k === 'sidebar' ? look.sidebar : '');
  function setColour(k, hex) {
    draft = { ...draft, colours: { ...draft.colours, [k]: hex } };
  }
  function typed(k, e) {
    const text = e.currentTarget.value.trim();
    if (text === '' && (k === 'onAccent' || MORE_COLOURS.includes(k))) return setColour(k, '');
    const hex = normalizeHex(text);
    if (hex) setColour(k, hex);
    else e.currentTarget.value = draft.colours[k];
  }
  let moreOpen = !!MORE_COLOURS.some((k) => draft.colours[k]);

  // ── Font ──
  $: fontOptions = [...Object.keys(FONTS), 'custom'].map((id) => ({ value: id, label: $t('themeEditor.fontName.' + id) }));
  // Whether this device has a font: a word drawn in it beside two different
  // fallbacks comes out the same width as neither of them.
  function hasFont(name) {
    if (!name) return false;
    const c = document.createElement('canvas').getContext('2d');
    const word = 'mmmmmmmmmmlli 0O';
    return ['monospace', 'serif'].some((fallback) => {
      c.font = `24px ${fallback}`;
      const plain = c.measureText(word).width;
      c.font = `24px "${name}", ${fallback}`;
      return c.measureText(word).width !== plain;
    });
  }
  $: customFound = draft.font === 'custom' && hasFont(draft.customFont);

  // ── Readability ──
  const fix = (id) => (draft = fixCheck(draft, id));
  const fixAll = () => {
    for (const c of failing) draft = fixCheck(draft, c.id);
  };

  // ── Done ──
  $: builtInNames = Object.keys(FAMILIES).map((f) => $t('theme.name.' + f));
  function save() {
    const a = sanitizeAppearance(get(appearance));
    const others = a.customThemes.filter((x) => x.id !== draft.id);
    const name = uniqueName(draft.name.trim() || $t('themeEditor.namePlaceholder'), others.map((x) => x.name), builtInNames);
    const saved = sanitizeTheme({ ...draft, name });
    const at = a.customThemes.findIndex((x) => x.id === draft.id);
    const list = at === -1 ? [...others, saved] : a.customThemes.map((x) => (x.id === saved.id ? saved : x));
    appearance.update((v) => ({ ...v, customThemes: list, family: customFamily(saved.id), accent: THEME_ACCENT }));
    toast($t(isNew ? 'themeEditor.savedNew' : 'themeEditor.saved', { name }), 'success');
    dispatch('done');
  }
  const exportIt = () => exportToFile(draft, $t);

  $: covers = $gameList.slice(0, 4);
  const pct = (v, { min, max }) => `${((v - min) / (max - min)) * 100}%`;
</script>

<svelte:window on:keydown={(e) => wheelFor && e.key === 'Escape' && (e.stopPropagation(), (wheelFor = null))} />

<div class="editor">
  <div class="head">
    <button class="back" title={$t('themeEditor.back')} aria-label={$t('themeEditor.back')} on:click={() => dispatch('done')}><ArrowLeft size={17} /></button>
    <div>
      <span class="crumb">{$t('settings.section.appearance')}</span>
      <h4>{isNew ? $t('themeEditor.create') : $t('themeEditor.edit', { name: theme.name })}</h4>
    </div>
  </div>

  <div class="split">
    <form class="form" on:submit|preventDefault={save}>
      <div class="pair">
        <label class="field">
          <span>{$t('themeEditor.name')}</span>
          <input bind:value={draft.name} placeholder={$t('themeEditor.namePlaceholder')} maxlength="40" spellcheck="false" />
        </label>
        <div class="field">
          <span>{$t('themeEditor.startFrom')}</span>
          <Select value={startFrom} options={startOptions} placeholder={$t('themeEditor.startFromPlaceholder')} label={$t('themeEditor.startFrom')} on:change={(e) => restart(e.detail)} />
        </div>
      </div>

      <section>
        <h5>{$t('themeEditor.colours')}</h5>
        <div class="colours">
          {#each COLOURS as k}
            <div class="crow">
              <span class="wheel-at">
                <button type="button" class="chip" class:auto={!draft.colours[k]} style="--c: {shown(k)}" aria-label={$t('themeEditor.pick', { what: $t('themeEditor.colour.' + k) })} on:click={() => (wheelFor = wheelFor === k ? null : k)}></button>
                {#if wheelFor === k}
                  <AccentWheel align="left" confirm={$t('colourWheel.use')} initial={shown(k)} on:pick={(e) => (setColour(k, e.detail), (wheelFor = null))} on:close={() => (wheelFor = null)} />
                {/if}
              </span>
              <span class="cwords"><span class="cname">{$t('themeEditor.colour.' + k)}</span><span class="chelp">{$t('themeEditor.colourHelp.' + k)}</span></span>
              <input class="hex" data-hex={k} value={draft.colours[k]} placeholder={k === 'onAccent' ? $t('themeEditor.auto') : ''} aria-label={$t('themeEditor.colour.' + k)} spellcheck="false" on:change={(e) => typed(k, e)} />
            </div>
          {/each}
        </div>

        <button type="button" class="more" class:open={moreOpen} on:click={() => (moreOpen = !moreOpen)}><ChevronDown size={15} />{$t('themeEditor.more')}</button>
        {#if moreOpen}
          <div class="colours" transition:collapse>
            {#each MORE_COLOURS as k}
              <div class="crow">
                <span class="wheel-at">
                  {#if shown(k)}
                    <button type="button" class="chip" class:auto={!draft.colours[k]} style="--c: {shown(k)}" aria-label={$t('themeEditor.pick', { what: $t('themeEditor.colour.' + k) })} on:click={() => (wheelFor = wheelFor === k ? null : k)}></button>
                  {:else}
                    <button type="button" class="chip none" aria-label={$t('themeEditor.pick', { what: $t('themeEditor.colour.' + k) })} on:click={() => (wheelFor = wheelFor === k ? null : k)}><Plus size={13} /></button>
                  {/if}
                  {#if wheelFor === k}
                    <AccentWheel align="left" confirm={$t('colourWheel.use')} initial={shown(k) || draft.colours.accent} on:pick={(e) => (setColour(k, e.detail), (wheelFor = null))} on:close={() => (wheelFor = null)} />
                  {/if}
                </span>
                <span class="cwords"><span class="cname">{$t('themeEditor.colour.' + k)}</span><span class="chelp">{$t('themeEditor.colourHelp.' + k)}</span></span>
                <input class="hex" data-hex={k} value={draft.colours[k]} placeholder={$t(k === 'sidebar' ? 'themeEditor.auto' : 'themeEditor.appsOwn')} aria-label={$t('themeEditor.colour.' + k)} spellcheck="false" on:change={(e) => typed(k, e)} />
                {#if draft.colours[k]}
                  <button type="button" class="reset" title={$t('themeEditor.resetColour')} aria-label={$t('themeEditor.resetColour')} on:click={() => setColour(k, '')}><RotateCcw size={14} /></button>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <section>
        <h5>{$t('themeEditor.shape')}</h5>
        {#each [['roundness', 'sharp', 'round'], ['borders', 'none', 'strong'], ['shadows', 'flat', 'deep']] as [k, lo, hi]}
          <div class="slider">
            <span class="sname">{$t('themeEditor.' + k)}</span>
            <span class="end">{$t('themeEditor.' + lo)}</span>
            <input type="range" min={SHAPE[k].min} max={SHAPE[k].max} step={SHAPE[k].step} bind:value={draft[k]} style="--at: {pct(draft[k], SHAPE[k])}" aria-label={$t('themeEditor.' + k)} />
            <span class="end">{$t('themeEditor.' + hi)}</span>
          </div>
        {/each}
      </section>

      <section>
        <h5>{$t('themeEditor.font')}</h5>
        <div class="pair">
          <Select bind:value={draft.font} options={fontOptions} label={$t('themeEditor.font')} />
          {#if draft.font === 'custom'}
            <input bind:value={draft.customFont} placeholder={$t('themeEditor.fontNamePlaceholder')} aria-label={$t('themeEditor.fontNamePlaceholder')} spellcheck="false" maxlength="60" />
          {/if}
        </div>
        {#if draft.font === 'custom' && draft.customFont}
          <p class="note" class:warn={!customFound}>
            {customFound ? $t('themeEditor.fontFound', { name: draft.customFont }) : $t('themeEditor.fontMissing', { name: draft.customFont })}
          </p>
        {/if}
      </section>
    </form>

    <aside>
      <h5>{$t('themeEditor.preview')}</h5>
      <!-- The app in this theme, small: what each colour does, where. -->
      <div class="pv" style={previewStyle} aria-hidden="true">
        <span class="pv-side">
          <span class="pv-logo"></span>
          <span class="pv-nav on"></span>
          <span class="pv-nav"></span>
          <span class="pv-nav"></span>
        </span>
        <span class="pv-main">
          <span class="pv-top">
            <span class="pv-title">{$t('themeEditor.pv.library')}</span>
            <span class="pv-btn">{$t('themeEditor.pv.addGame')}</span>
          </span>
          <span class="pv-covers">
            {#each [0, 1, 2, 3] as i}<span class="pv-cv">{#if covers[i]}<CoverImage src={gameCover(covers[i], true)} alt="" />{/if}</span>{/each}
          </span>
          <span class="pv-card">
            <span class="pv-line"><b>{$t('themeEditor.pv.synced')}</b><span class="pv-chip">{$t('themeEditor.pv.online')}</span></span>
            <span class="pv-quiet">{$t('themeEditor.pv.quiet')}</span>
            <span class="pv-faint">{$t('themeEditor.pv.faint')}</span>
          </span>
        </span>
      </div>

      <h5 class="gap">{$t('themeEditor.readability')}</h5>
      <div class="checks">
        {#each checks as c (c.id)}
          <div class="chk" class:bad={!c.ok}>
            <span class="ci">{#if c.ok}<Check size={14} />{:else}<TriangleAlert size={14} />{/if}</span>
            <span class="cl">{$t('themeEditor.check.' + c.id)}</span>
            <span class="cr">{c.ratio.toFixed(1)}:1</span>
            {#if !c.ok}<button type="button" class="btn small" on:click={() => fix(c.id)}>{$t('themeEditor.fix')}</button>{/if}
          </div>
        {/each}
      </div>
      <p class="note">{$t('themeEditor.readabilityNote')}</p>
    </aside>
  </div>

  <div class="foot">
    <button type="button" class="btn ghost" on:click={exportIt}><Download size={15} />{$t('themeEditor.export')}</button>
    {#if failing.length}
      <span class="warn-line"><TriangleAlert size={14} />{$t('themeEditor.saveWarning')}<button type="button" class="link" on:click={fixAll}>{$t('themeEditor.fixAll')}</button></span>
    {/if}
    <span class="grow"></span>
    <button type="button" class="btn" on:click={() => dispatch('done')}>{$t('common.cancel')}</button>
    <button type="button" class="btn primary" on:click={save}>{$t('themeEditor.save')}</button>
  </div>
</div>

<style>
  .editor {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .back {
    width: 34px;
    height: 34px;
    display: grid;
    place-items: center;
    border: 1px solid var(--border);
    border-radius: calc(9px * var(--round, 1));
    background: var(--bg-raised);
    color: var(--text-dim);
    cursor: pointer;
  }
  .back:hover {
    color: var(--text);
    border-color: var(--border-strong);
  }
  .crumb {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  h4 {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  h5 {
    margin-bottom: 8px;
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  h5.gap {
    margin-top: 16px;
  }
  .split {
    display: flex;
    gap: 22px;
    align-items: flex-start;
  }
  .form {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  aside {
    flex: none;
    width: 300px;
    position: sticky;
    top: 0;
  }
  .pair {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin: 0;
  }
  .field > span {
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-dim);
  }

  /* Each colour: its swatch, what it is for, its hex. */
  .colours {
    display: flex;
    flex-direction: column;
    border-radius: calc(12px * var(--round, 1));
    background: var(--bg-raised);
    border: 1px solid var(--border);
  }
  .crow {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 9px 12px;
    border-top: 1px solid var(--border);
  }
  .crow:first-child {
    border-top: 0;
  }
  .wheel-at {
    position: relative;
    display: inline-flex;
  }
  .chip {
    width: 30px;
    height: 30px;
    padding: 0;
    border: 0;
    border-radius: calc(8px * var(--round, 1));
    background: var(--c);
    box-shadow: inset 0 0 0 1px rgba(127, 127, 127, 0.35);
    cursor: pointer;
    transition: transform 0.15s cubic-bezier(0.2, 0.7, 0.2, 1.4);
  }
  .chip:hover {
    transform: scale(1.07);
  }
  .chip.auto {
    background:
      linear-gradient(135deg, transparent 46%, rgba(127, 127, 127, 0.55) 47%, rgba(127, 127, 127, 0.55) 53%, transparent 54%),
      var(--c);
  }
  .chip.none {
    display: grid;
    place-items: center;
    background: var(--bg);
    color: var(--text-faint);
    box-shadow: inset 0 0 0 1px var(--border-strong);
  }
  .cwords {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .cname {
    font-weight: 600;
    font-size: var(--text-md);
  }
  .chelp {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .hex {
    width: 104px;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: var(--text-sm);
    text-transform: lowercase;
  }
  .reset {
    width: 28px;
    height: 28px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: calc(7px * var(--round, 1));
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  .reset:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .more {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin: 10px 0 8px;
    padding: 4px 2px;
    border: 0;
    background: transparent;
    color: var(--accent-text);
    font: inherit;
    font-size: var(--text-md);
    font-weight: 600;
    cursor: pointer;
  }
  .more :global(svg) {
    transition: transform 0.2s;
  }
  .more.open :global(svg) {
    transform: rotate(180deg);
  }

  /* Shape: three sliders, each between its two ends. */
  .slider {
    display: grid;
    grid-template-columns: 96px auto 1fr auto;
    align-items: center;
    gap: 10px;
    padding: 6px 0;
  }
  .sname {
    font-weight: 600;
    font-size: var(--text-md);
  }
  .end {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  input[type='range'] {
    appearance: none;
    -webkit-appearance: none;
    height: 6px;
    border-radius: 999px;
    background: linear-gradient(90deg, var(--accent) var(--at), var(--bg-active) var(--at));
    cursor: pointer;
  }
  input[type='range']::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: #fff;
    box-shadow:
      0 0 0 1px rgba(0, 0, 0, 0.25),
      0 2px 6px rgba(0, 0, 0, 0.35);
    transition: transform 0.15s;
  }
  input[type='range']:active::-webkit-slider-thumb {
    transform: scale(1.15);
  }
  .note {
    margin-top: 8px;
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .note.warn {
    color: var(--warn);
  }

  /* The picture: the app, in the theme being made. */
  .pv {
    display: flex;
    height: 200px;
    overflow: hidden;
    border-radius: calc(12px * var(--round, 1));
    background: var(--bg);
    color: var(--text);
    box-shadow: inset 0 0 0 1px var(--border-strong);
    transition: background 0.25s;
  }
  .pv-side {
    width: 52px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: 12px 0;
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
  }
  .pv-logo {
    width: 22px;
    height: 22px;
    border-radius: calc(6px * var(--round, 1));
    background: var(--accent);
    margin-bottom: 6px;
  }
  .pv-nav {
    width: 26px;
    height: 8px;
    border-radius: calc(4px * var(--round, 1));
    background: var(--bg-active);
  }
  .pv-nav.on {
    background: var(--accent);
    opacity: 0.8;
  }
  .pv-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 9px;
    padding: 12px;
  }
  .pv-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .pv-title {
    font-weight: 700;
    font-size: 15px;
  }
  .pv-btn {
    padding: 4px 9px;
    border-radius: calc(7px * var(--round, 1));
    background: var(--accent-fill);
    color: var(--on-accent);
    font-size: 11px;
    font-weight: 600;
  }
  .pv-covers {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 6px;
    height: 62px;
  }
  .pv-cv {
    position: relative;
    overflow: hidden;
    border-radius: calc(6px * var(--round, 1));
    background: var(--bg-active);
    box-shadow: var(--pv-shadow);
  }
  .pv-cv :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .pv-card {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 8px 10px;
    border-radius: calc(10px * var(--round, 1));
    background: var(--bg-raised);
    border: 1px solid var(--border);
    box-shadow: var(--pv-shadow);
    font-size: 11px;
  }
  .pv-line {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
  }
  .pv-chip {
    padding: 1px 7px;
    border-radius: 999px;
    background: var(--bg-active);
    color: var(--text-dim);
    font-size: 10px;
  }
  .pv-quiet {
    color: var(--text-dim);
  }
  .pv-faint {
    color: var(--text-faint);
  }

  /* Readability: each pair, how it reads, and Fix where it does not. */
  .checks {
    display: flex;
    flex-direction: column;
    border-radius: calc(10px * var(--round, 1));
    background: var(--bg-raised);
    border: 1px solid var(--border);
  }
  .chk {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 38px;
    padding: 6px 10px;
    border-top: 1px solid var(--border);
    font-size: var(--text-md);
  }
  .chk:first-child {
    border-top: 0;
  }
  .ci {
    display: grid;
    color: var(--success);
  }
  .bad .ci {
    color: var(--warn);
  }
  .cl {
    flex: 1;
    min-width: 0;
  }
  .cr {
    font-variant-numeric: tabular-nums;
    color: var(--text-faint);
    font-size: var(--text-sm);
  }

  .foot {
    display: flex;
    align-items: center;
    gap: 10px;
    padding-top: 14px;
    border-top: 1px solid var(--border);
  }
  .grow {
    flex: 1;
  }
  .warn-line {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: var(--text-sm);
    color: var(--warn);
  }
  .link {
    margin-left: 4px;
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent-text);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }

  @media (max-width: 1000px) {
    .split {
      flex-direction: column;
    }
    aside {
      width: 100%;
      position: static;
    }
  }
</style>
