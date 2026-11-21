<script>
  // Settings, as a menu over the app (the SettingsV3 board): the sections
  // down the left with a search over every setting, the open section on the
  // right as rows — what each is, its control beside it. It opens over
  // whatever page you are on and closes back to it (lib/settingsmenu.js).
  //
  // Every change saves itself (lib/autosave.js): a switch or a choice at
  // once, typed text when you leave the box or press Enter. `draft` is what
  // the menu shows and edits; `saved` is what the daemon last confirmed. A
  // value it refused stays marked, with why, until it is changed.
  //
  // Motion: it opens as every dialog does; the marker slides to the section
  // chosen and the section rises in; a setting found by search is lit.
  import { onMount, onDestroy, tick } from 'svelte';
  import { settings } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';
  import { t } from '../lib/i18n.js';
  import { adoptOutsideChanges, changedFields, createAutosave } from '../lib/autosave.js';
  import { settingsMenu, closeSettings, SECTIONS, searchSettings } from '../lib/settingsmenu.js';
  import { holdsDialog } from '../lib/dialogs.js';
  import { dialogOut, slidingIndicator, rise } from '../lib/motion.js';
  import { backdropClose } from '../lib/backdrop.js';
  import Skeleton from './ui/Skeleton.svelte';
  import GeneralSection from '../views/settings/sections/GeneralSection.svelte';
  import AppearanceSection from '../views/settings/sections/AppearanceSection.svelte';
  import SyncSection from '../views/settings/sections/SyncSection.svelte';
  import DevicesSection from '../views/settings/sections/DevicesSection.svelte';
  import CloudSection from '../views/settings/sections/CloudSection.svelte';
  import NotificationsSection from '../views/settings/sections/NotificationsSection.svelte';
  import StorageSection from '../views/settings/sections/StorageSection.svelte';
  import ControllerSection from '../views/settings/sections/ControllerSection.svelte';
  import AdvancedSection from '../views/settings/sections/AdvancedSection.svelte';
  import AboutSection from '../views/settings/sections/AboutSection.svelte';
  import X from 'lucide-svelte/icons/x';
  import Search from 'lucide-svelte/icons/search';
  import Check from 'lucide-svelte/icons/check';
  import LoaderCircle from 'lucide-svelte/icons/loader-circle';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import SlidersHorizontal from 'lucide-svelte/icons/sliders-horizontal';
  import Palette from 'lucide-svelte/icons/palette';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import MonitorSmartphone from 'lucide-svelte/icons/monitor-smartphone';
  import Cloud from 'lucide-svelte/icons/cloud';
  import Bell from 'lucide-svelte/icons/bell';
  import Database from 'lucide-svelte/icons/database';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Wrench from 'lucide-svelte/icons/wrench';
  import Info from 'lucide-svelte/icons/info';

  const ICONS = {
    general: SlidersHorizontal,
    appearance: Palette,
    sync: RefreshCw,
    devices: MonitorSmartphone,
    cloud: Cloud,
    notifications: Bell,
    storage: Database,
    controller: Gamepad2,
    advanced: Wrench,
    about: Info
  };
  const VIEWS = {
    general: GeneralSection,
    appearance: AppearanceSection,
    sync: SyncSection,
    devices: DevicesSection,
    cloud: CloudSection,
    notifications: NotificationsSection,
    storage: StorageSection,
    controller: ControllerSection,
    advanced: AdvancedSection,
    about: AboutSection
  };

  // What each section is given: the settings to edit (bound, so a change
  // there is a change here), how saving stands, and the means to save. A
  // section is given only what it takes — binding `draft` to one that has no
  // such prop hands back nothing, and the settings with it.
  const EDITS = new Set(['general', 'sync', 'devices', 'storage', 'advanced']);
  const TAKES = {
    general: ['unsaved', 'save', 'appVersion'],
    sync: ['save'],
    devices: ['unsaved', 'save'],
    storage: ['unsaved', 'save', 'flush'],
    advanced: ['unsaved', 'save'],
    about: ['appVersion']
  };
  $: given = Object.fromEntries((TAKES[section] ?? []).map((k) => [k, { unsaved, save: saveNow, flush, appVersion }[k]]));

  $: section = $settingsMenu?.section ?? 'general';

  let appVersion = '';
  onMount(async () => {
    try {
      appVersion = (await native.appInfo())?.version ?? '';
    } catch {
      appVersion = '';
    }
  });

  // ── Saving ──
  let draft = null;
  let saved = null;
  let saveState = null; // null | 'saving' | 'saved' | {error}
  let rejected = {};
  let refusedWhy = {};
  const FIELD_NAMES = {
    relayUrl: 'settings.field.relayUrl',
    relayPort: 'settings.field.relayPort',
    port: 'settings.field.port',
    deviceName: 'settings.field.deviceName',
    backupsDir: 'settings.field.backupsDir',
    syncBackupsDir: 'settings.field.syncBackupsDir',
    pathTranslations: 'settings.field.pathTranslations'
  };
  const fieldName = (key) => (key in FIELD_NAMES ? $t(FIELD_NAMES[key]) : key);
  const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);
  $: unsaved = draft ? Object.keys(rejected).filter((key) => same(draft[key], rejected[key])) : [];
  const firstSentence = (text) => String(text).split(/(?<=\.)\s/)[0];

  // The settings as the menu edits them. The cloud's are set on the Backups
  // page, and left out: they are not the menu's to send.
  function editable(s) {
    const { cloudSync, cloudAutoPull, ...rest } = structuredClone(s);
    rest.defaultMaxManualSnapshots ??= 0;
    rest.updateChannel ??= 'stable';
    return rest;
  }
  // A path rule with a side still empty is not a rule yet: it stays in the
  // menu but is not saved until both sides are filled in.
  const ready = (d) => ({
    ...d,
    pathTranslations: (d.pathTranslations ?? []).filter((r) => r.fromPattern?.trim() && r.toPattern?.trim())
  });

  const saver = createAutosave({
    collect: () => changedFields(saved, ready(draft), { rejected }),
    send: (patch) => api.post('/api/settings', patch),
    onSaved(result, patch) {
      const next = editable(result);
      for (const key of Object.keys(patch)) {
        if (same(draft[key], patch[key])) draft[key] = structuredClone(next[key]);
        delete rejected[key];
        delete refusedWhy[key];
      }
      rejected = rejected;
      saved = next;
      settings.set(result);
    },
    onFailed(patch, error) {
      rejected = { ...rejected, ...patch };
      for (const key of Object.keys(patch)) refusedWhy[key] = error.message;
    },
    onState: (state) => (saveState = state)
  });
  const saveNow = () => saver.flush();
  // For a section that acts on what is saved: saves, then says how it went.
  async function flush() {
    await saver.flush();
    return saveState && typeof saveState === 'object' ? saveState : {};
  }
  onDestroy(saveNow);

  // Arrivals from elsewhere while the menu is open — the tray, the terminal,
  // another device — are taken in, except into a field being edited here.
  function takeSettings(s) {
    if (!s) return;
    if (!draft) {
      saved = editable(s);
      draft = structuredClone(saved);
      return;
    }
    ({ saved, draft } = adoptOutsideChanges(saved, draft, editable(s)));
  }
  $: takeSettings($settings);

  // ── Moving about ──
  function show(next) {
    saveNow();
    query = '';
    settingsMenu.update((m) => ({ ...m, section: next, focus: null }));
  }
  function close() {
    saveNow();
    closeSettings();
  }

  // ── Search ──
  let query = '';
  let searchEl;
  $: results = searchSettings(query, $t);
  function goTo(entry) {
    query = '';
    saveNow();
    settingsMenu.set({ section: entry.section, focus: entry.id });
  }
  // A setting asked for — a search result, a link to one — is brought into
  // view and lit for a moment.
  let body;
  $: if ($settingsMenu?.focus && draft) reveal($settingsMenu.focus);
  async function reveal(id) {
    await tick();
    const row = body?.querySelector(`#set-${CSS.escape(id)}`);
    if (!row) return;
    row.scrollIntoView({ block: 'center', behavior: 'smooth' });
    row.classList.add('found');
    setTimeout(() => row.classList.remove('found'), 1400);
    settingsMenu.update((m) => (m ? { ...m, focus: null } : m));
  }

  function keys(e) {
    if (e.key === 'Escape') {
      e.stopPropagation();
      if (query) query = '';
      else close();
    } else if (e.key === '/' && !e.target.closest('input, textarea, select')) {
      e.preventDefault();
      searchEl?.focus();
    }
  }
  let panel;
  onMount(() => {
    if (!panel.contains(document.activeElement)) panel.focus();
  });
</script>

<div class="overlay" out:dialogOut|global use:backdropClose={close} role="presentation" on:keydown={keys}>
  <div class="menu" bind:this={panel} tabindex="-1" role="dialog" aria-modal="true" use:holdsDialog aria-labelledby="settings-title">
    <nav aria-label={$t('settings.title')}>
      <h2 id="settings-title">{$t('settings.title')}</h2>
      <label class="search">
        <Search size={15} />
        <input bind:this={searchEl} bind:value={query} placeholder={$t('settings.menu.search')} aria-label={$t('settings.menu.search')} spellcheck="false" />
        {#if query}
          <button class="clear" aria-label={$t('common.close')} on:click={() => (query = '')}><X size={13} /></button>
        {:else}
          <kbd>/</kbd>
        {/if}
      </label>
      <div class="sections" use:slidingIndicator={{ mode: 'fill', className: 'nav-mark' }}>
        {#each SECTIONS as id}
          <button class:on={!query && section === id} aria-current={!query && section === id ? 'page' : null} on:click={() => show(id)}>
            <svelte:component this={ICONS[id]} size={16} />{$t('settings.section.' + id)}
          </button>
        {/each}
      </div>
      <div class="foot">
        <span>OpenSave {appVersion}</span>
      </div>
    </nav>

    <div class="pane">
      <header>
        <div class="titles">
          {#if query}
            <h3>{$t('settings.menu.search')}</h3>
          {:else}
            {#key section}
              <h3 in:rise={{ y: 4 }}>{$t('settings.section.' + section)}</h3>
              <p in:rise={{ y: 4, delay: 30 }}>{$t(`settings.section.${section}Hint`)}</p>
            {/key}
          {/if}
        </div>
        {#if draft}
          <span class="state" class:error={saveState?.error || unsaved.length} aria-live="polite">
            {#if saveState === 'saving'}
              <span class="spin"><LoaderCircle size={14} /></span>{$t('settings.saveState.saving')}
            {:else if unsaved.length}
              <TriangleAlert size={14} />
              <span class="why" title={refusedWhy[unsaved[0]]}>{$t('settings.saveState.notSaved', { field: fieldName(unsaved[0]), why: firstSentence(refusedWhy[unsaved[0]]) })}</span>
            {:else if saveState?.error}
              <TriangleAlert size={14} /><span class="why" title={saveState.error}>{$t('settings.saveState.saveFailed', { error: firstSentence(saveState.error) })}</span>
            {:else if saveState === 'saved'}
              <Check size={14} />{$t('settings.saveState.saved')}
            {/if}
          </span>
        {/if}
        <button class="shut" title={$t('settings.menu.close')} aria-label={$t('settings.menu.close')} on:click={close}><X size={18} /></button>
      </header>

      <div class="body" bind:this={body} on:change={saveNow} on:keydown={(e) => e.key === 'Enter' && e.target.matches('input') && saveNow()} role="presentation">
        {#if !draft}
          <Skeleton kind="cards" count={3} />
        {:else if query}
          <div class="results">
            {#each results as r, i (r.section + r.id)}
              <button class="result" style="--i: {Math.min(i, 8)}" on:click={() => goTo(r)}>
                <span class="r-icon"><svelte:component this={ICONS[r.section]} size={15} /></span>
                <span class="r-words">
                  <span class="r-name">{$t(r.label)}</span>
                  <span class="r-where">{$t('settings.menu.found', { section: $t('settings.section.' + r.section) })}</span>
                </span>
              </button>
            {:else}
              <p class="none">{$t('settings.menu.noResults', { q: query })}</p>
            {/each}
          </div>
        {:else}
          {#key section}
            <div class="section">
              {#if EDITS.has(section)}
                <svelte:component this={VIEWS[section]} bind:draft {...given} />
              {:else}
                <svelte:component this={VIEWS[section]} {...given} />
              {/if}
            </div>
          {/key}
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 85;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
    background: var(--overlay);
  }
  .menu {
    width: min(1080px, 100%);
    height: min(720px, 100%);
    display: flex;
    overflow: hidden;
    outline: none;
    border-radius: calc(16px * var(--round, 1));
    background: var(--bg);
    border: 1px solid var(--border-strong);
    box-shadow: 0 30px 90px rgba(0, 0, 0, 0.6);
  }

  /* The sections, down the left. */
  nav {
    flex: none;
    width: 252px;
    display: flex;
    flex-direction: column;
    gap: 14px;
    padding: 20px 14px 14px;
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
  }
  h2 {
    padding: 0 6px;
    font-size: var(--text-xl);
    font-weight: 700;
  }
  .search {
    display: flex;
    align-items: center;
    gap: 9px;
    height: 38px;
    padding: 0 8px 0 11px;
    border-radius: calc(9px * var(--round, 1));
    border: 1px solid var(--border-strong);
    background: var(--bg);
    color: var(--text-faint);
    transition: border-color 0.15s;
  }
  .search:focus-within {
    border-color: var(--accent);
  }
  .search input {
    flex: 1;
    min-width: 0;
    height: 100%;
    padding: 0;
    border: 0;
    outline: 0;
    background: transparent;
    color: var(--text);
    font-size: var(--text-md);
  }
  kbd {
    padding: 1px 6px;
    border: 1px solid var(--border-strong);
    border-radius: calc(4px * var(--round, 1));
    font: inherit;
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .clear {
    width: 22px;
    height: 22px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: calc(6px * var(--round, 1));
    background: var(--bg-hover);
    color: var(--text-dim);
    cursor: pointer;
  }
  .sections {
    display: flex;
    flex-direction: column;
    gap: 2px;
    overflow-y: auto;
  }
  .sections button {
    position: relative;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: 12px;
    height: 38px;
    padding: 0 12px;
    border: 0;
    border-radius: calc(8px * var(--round, 1));
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-base);
    text-align: left;
    cursor: pointer;
    transition: color 0.15s;
  }
  .sections button:hover {
    color: var(--text);
  }
  .sections button.on {
    color: var(--text);
    font-weight: 600;
  }
  .sections button.on :global(svg) {
    color: var(--accent-text);
  }
  .sections :global(.nav-mark) {
    border-radius: calc(8px * var(--round, 1));
    background: var(--bg-active);
    box-shadow: inset 0 0 0 1px var(--border-strong);
  }
  .sections:not(:global(.slides)) button.on {
    background: var(--bg-active);
  }
  .foot {
    margin-top: auto;
    padding: 0 6px;
    font-size: var(--text-sm);
    color: var(--text-faint);
  }

  /* The section open, on the right. */
  .pane {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  header {
    flex: none;
    display: flex;
    align-items: flex-start;
    gap: 16px;
    padding: 20px 20px 16px 26px;
    border-bottom: 1px solid var(--border);
  }
  .titles {
    flex: 1;
    min-width: 0;
  }
  h3 {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .titles p {
    margin-top: 2px;
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .state {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    max-width: 40%;
    padding-top: 6px;
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .state.error {
    color: var(--warn);
  }
  .why {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .spin {
    display: grid;
    animation: turn 1s linear infinite;
  }
  @keyframes turn {
    to {
      transform: rotate(360deg);
    }
  }
  .shut {
    flex: none;
    width: 34px;
    height: 34px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: calc(9px * var(--round, 1));
    background: transparent;
    color: var(--text-dim);
    cursor: pointer;
  }
  .shut:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 20px 26px 28px;
  }
  .section {
    display: flex;
    flex-direction: column;
    gap: 22px;
    animation: section-in 0.3s cubic-bezier(0.2, 0.8, 0.2, 1) backwards;
  }
  @keyframes section-in {
    from {
      transform: translateY(8px);
    }
  }

  /* Search, in place of the section: every setting it found, and where. */
  .results {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .result {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: calc(10px * var(--round, 1));
    background: var(--bg-raised);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    animation: section-in 0.26s cubic-bezier(0.2, 0.8, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 22ms);
    transition: border-color 0.15s;
  }
  .result:hover {
    border-color: var(--border-strong);
  }
  .r-icon {
    width: 30px;
    height: 30px;
    display: grid;
    place-items: center;
    border-radius: calc(8px * var(--round, 1));
    background: var(--bg-active);
    color: var(--accent-text);
  }
  .r-words {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .r-name {
    font-weight: 600;
  }
  .r-where {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .none {
    padding: 24px 4px;
    color: var(--text-dim);
  }

  /* A narrow window: the sections take less room. */
  @media (max-width: 820px) {
    nav {
      width: 200px;
    }
  }
</style>
