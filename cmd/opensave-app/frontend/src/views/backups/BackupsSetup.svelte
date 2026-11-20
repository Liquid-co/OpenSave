<script>
  // Choosing where cloud backups go: the storage you already have, as cards
  // with each one's own logo; then signing in, in numbered steps, or the
  // folder or server to use. Fires `done` once backups have somewhere to go
  // and are on, `back` to return to the page without changing anything.
  import { createEventDispatcher, onDestroy } from 'svelte';
  import ArrowLeft from 'lucide-svelte/icons/arrow-left';
  import ShieldCheck from 'lucide-svelte/icons/shield-check';
  import Folder from 'lucide-svelte/icons/folder';
  import Server from 'lucide-svelte/icons/server';
  import Check from 'lucide-svelte/icons/check';
  import LoaderCircle from 'lucide-svelte/icons/loader-circle';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import { api, native } from '../../lib/api.js';
  import { toast, settings, cloudAuthEvent } from '../../lib/stores.js';
  import { providerById, connectedProviderOf, isEmail, needsOwnApp } from '../../lib/cloudproviders.js';
  import { t } from '../../lib/i18n.js';
  import { rise } from '../../lib/motion.js';
  import ProviderIcon from '../cloud/ProviderIcon.svelte';
  import OwnOAuthApp from '../cloud/OwnOAuthApp.svelte';
  import FolderBrowser from '../../components/FolderBrowser.svelte';
  import { writable } from 'svelte/store';

  export let config;
  export let canGoBack = false;
  const dispatch = createEventDispatcher();

  const SIGN_IN = ['google_drive', 'onedrive', 'dropbox'];
  let step = 'choose'; // 'choose' | a provider id
  $: connected = connectedProviderOf(config);
  const busy = writable(false);

  // ── Saving where ──
  async function save(patch) {
    busy.set(true);
    try {
      settings.set(await api.post('/api/settings', { cloudSync: { ...patch, enabled: true } }));
      toast($t('backups.setup.saved', { provider: $t('cloud.provider.' + patch.provider) }), 'success');
      dispatch('done');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy.set(false);
    }
  }

  // ── Your own storage ──
  let url = '';
  let username = '';
  let password = '';
  let headers = '{}';
  let browsing = false;
  function choose(id) {
    step = id;
    const same = config.provider === id;
    url = same ? (config.url ?? '') : '';
    username = same ? (config.username ?? '') : '';
    password = same ? (config.password ?? '') : '';
    headers = same ? config.headers || '{}' : '{}';
    folderId = config.folderId ?? '';
    started = false;
    browsing = false;
    // Already signed in to this one: nothing to do but use it.
    if (SIGN_IN.includes(id) && connected === id) save({ provider: id });
  }
  async function pickFolder() {
    if (!native.isWails()) {
      browsing = !browsing;
      return;
    }
    const dir = await native.selectDirectory($t('cloud.fields.selectFolderDialog'));
    if (dir) url = dir;
  }

  // ── Signing in ──
  let started = false;
  let auto = false;
  let pasted = '';
  let folderId = '';
  async function startSignIn() {
    busy.set(true);
    try {
      const res = await api.post('/api/auth/start', { provider: step });
      native.openExternal(res.authUrl);
      started = true;
      auto = !!res.autoCallback;
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy.set(false);
    }
  }
  async function signedIn(email) {
    toast($t('cloud.connect.connectedAs', { email }), 'success');
    await save({ provider: step, ...(step === 'google_drive' ? { folderId: folderId.trim() } : {}) });
  }
  // The daemon says when the browser came back to it (cloud-auth).
  const unsub = cloudAuthEvent.subscribe((ev) => {
    if (!ev || !started) return;
    cloudAuthEvent.set(null);
    if (ev.success) signedIn(ev.userEmail);
    else toast(ev.error ?? $t('cloud.connect.signInFailed'), 'error');
  });
  onDestroy(unsub);
  async function finishPasted() {
    busy.set(true);
    try {
      const res = await api.post('/api/auth/callback', { code: pasted.trim() });
      pasted = '';
      await signedIn(res.userEmail);
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy.set(false);
    }
  }

  $: provider = providerById(step);
  $: ownCfg = { ...config, provider: step };
</script>

<div class="setup">
  {#if step === 'choose'}
    {#if canGoBack}<button class="btn ghost back" on:click={() => dispatch('back')}><ArrowLeft size={16} />{$t('backups.title')}</button>{/if}
    <div class="choose" in:rise={{ y: 6 }}>
      <div class="intro">
        <h1>{$t('backups.setup.title')}</h1>
        <p>{$t('backups.setup.sub')}</p>
      </div>
      <div class="big-row">
        {#each SIGN_IN as id}
          <button class="big" class:current={connected === id} on:click={() => choose(id)}>
            <span class="logo"><ProviderIcon provider={providerById(id)} size={40} /></span>
            <span>
              <span class="pname">{$t('cloud.provider.' + id)}</span>
              <span class="psub">
                {#if connected === id}<Check size={13} />{isEmail(config.tokens?.userEmail) ? config.tokens.userEmail : $t('cloud.picker.connected')}{:else}{$t('backups.setup.signInWith', { who: $t('backups.setup.who.' + id) })}{/if}
              </span>
            </span>
          </button>
        {/each}
      </div>
      <div>
        <div class="own-label">{$t('backups.setup.own')}</div>
        <div class="small-row">
          <button class="small" class:current={config.provider === 'local' && config.url} on:click={() => choose('local')}>
            <span class="st"><Folder size={20} /></span>
            <span><span class="pname">{$t('backups.setup.local')}</span><span class="psub">{$t('backups.setup.localSub')}</span></span>
          </button>
          <button class="small" class:current={config.provider === 'webdav' && config.url} on:click={() => choose('webdav')}>
            <span class="st"><Server size={20} /></span>
            <span><span class="pname">{$t('backups.setup.webdav')}</span><span class="psub">{$t('backups.setup.webdavSub')}</span></span>
          </button>
        </div>
      </div>
      <div class="assure">
        <ShieldCheck size={20} />
        <span>{$t('backups.setup.assure')}</span>
        <button class="btn ghost small" on:click={() => choose('webhook')}>{$t('backups.setup.webhook')}</button>
      </div>
    </div>
  {:else}
    <button class="btn ghost back" on:click={() => (step = 'choose')}><ArrowLeft size={16} />{$t('backups.setup.elsewhere')}</button>
    <section class="panel" in:rise={{ y: 6 }} aria-labelledby="setup-title">
      <div class="panel-head">
        <span class="logo big-logo" class:plain={!provider?.img}><ProviderIcon {provider} size={provider?.img ? 46 : 30} /></span>
        {#if SIGN_IN.includes(step)}
          <h1 id="setup-title">{$t('backups.setup.signInTo', { provider: $t('cloud.provider.' + step) })}</h1>
          <p>{step === 'google_drive' ? $t('backups.setup.driveScope') : $t('backups.setup.direct', { provider: $t('cloud.provider.' + step) })}</p>
        {:else}
          <h1 id="setup-title">{$t('backups.setup.' + step + 'Title')}</h1>
          <p>{$t('backups.setup.' + step + 'Panel')}</p>
        {/if}
      </div>

      {#if SIGN_IN.includes(step)}
        {#if needsOwnApp(step)}
          <!-- OneDrive has no built-in app: your own registration comes first. -->
          {#key step}<OwnOAuthApp config={ownCfg} {connected} {busy} reload={async () => {}} />{/key}
        {/if}
        <ol class="steps">
          <li class:done={started}>
            <span class="num">{#if started}<Check size={14} strokeWidth={3} />{:else}1{/if}</span>
            <div class="step-body">
              {#if started}
                <b>{$t('backups.setup.browserOpen')}</b>
              {:else}
                <b>{$t('backups.setup.openBrowser')}</b>
                <button class="btn primary" disabled={$busy} on:click={startSignIn}>{$t('cloud.connect.signInWith', { provider: $t('cloud.provider.' + step) })}</button>
              {/if}
            </div>
          </li>
          <li class:now={started}>
            <span class="num">2</span>
            <div class="step-body">
              <b>{$t('backups.setup.allow')}</b>
              <span class="step-sub">{auto ? $t('backups.setup.allowAuto') : $t('backups.setup.allowManual')}</span>
              {#if step === 'google_drive'}<span class="step-sub warn">{$t('cloud.connect.googleConsent1')}<strong>{$t('cloud.connect.googleConsentTick')}</strong>{$t('cloud.connect.googleConsent2')}</span>{/if}
              {#if started}
                <div class="waiting"><span class="spin"><LoaderCircle size={18} /></span><span>{$t('backups.setup.waitingFor', { provider: $t('cloud.provider.' + step) })}</span><button class="btn ghost small" on:click={startSignIn}>{$t('backups.setup.openAgain')}</button></div>
              {/if}
            </div>
          </li>
          <li class="later">
            <span class="num">3</span>
            <div class="step-body"><b>{$t('backups.setup.firstBackup')}</b><span class="step-sub">{$t('backups.setup.firstBackupSub')}</span></div>
          </li>
        </ol>
        <details class="fallback" open={started && !auto}>
          <summary>{$t('backups.setup.fallback')}</summary>
          <label for="setup-paste">{$t('backups.setup.paste')}</label>
          <div class="row">
            <input id="setup-paste" placeholder="http://localhost/callback?code=…" bind:value={pasted} />
            <button class="btn primary" disabled={$busy || !pasted.trim()} on:click={finishPasted}>{$t('backups.setup.continue')}</button>
          </div>
        </details>
        {#if !needsOwnApp(step)}
          {#key step}<OwnOAuthApp config={ownCfg} {connected} {busy} reload={async () => {}} />{/key}
        {/if}
        {#if step === 'google_drive'}
          <details class="fallback">
            <summary>{$t('cloud.backup.driveFolderIdLabel')}</summary>
            <input placeholder={$t('cloud.backup.driveFolderIdPlaceholder')} bind:value={folderId} />
            <span class="step-sub">{$t('cloud.backup.driveFolderIdHint')}</span>
          </details>
        {/if}
      {:else if step === 'local'}
        <div class="field">
          <label for="setup-folder">{$t('cloud.fields.destFolder')}</label>
          <div class="row">
            <input id="setup-folder" placeholder="D:\Backups\OpenSave" bind:value={url} />
            <button class="btn" class:on={browsing} on:click={pickFolder}><FolderOpen size={15} />{$t('folders.browse')}</button>
          </div>
        </div>
        {#if browsing}<FolderBrowser start={url} on:pick={(e) => { url = e.detail; browsing = false; }} />{/if}
        <button class="btn primary wide" disabled={$busy || !url.trim()} on:click={() => save({ provider: 'local', url: url.trim() })}>{$t('backups.setup.useFolder')}</button>
      {:else if step === 'webdav'}
        <label class="field"><span>{$t('cloud.fields.webdavUrl')}</span><input placeholder="https://nas.local/dav/opensave/" bind:value={url} /></label>
        <div class="two">
          <label class="field"><span>{$t('cloud.fields.username')}</span><input bind:value={username} autocomplete="off" /></label>
          <label class="field"><span>{$t('cloud.fields.password')}</span><input type="password" bind:value={password} autocomplete="off" /></label>
        </div>
        <button class="btn primary wide" disabled={$busy || !url.trim()} on:click={() => save({ provider: 'webdav', url: url.trim(), username, password })}>{$t('backups.setup.useServer')}</button>
      {:else if step === 'webhook'}
        <label class="field"><span>{$t('cloud.fields.webhookUrl')}</span><input bind:value={url} /></label>
        <label class="field"><span>{$t('cloud.fields.customHeaders')}</span><input placeholder={'{"Authorization": "Bearer …"}'} bind:value={headers} /></label>
        <p class="step-sub">{$t('backups.setup.webhookNote')}</p>
        <button class="btn primary wide" disabled={$busy || !url.trim()} on:click={() => save({ provider: 'webhook', url: url.trim(), headers })}>{$t('backups.setup.useAddress')}</button>
      {/if}
    </section>
  {/if}
</div>

<style>
  .setup {
    display: flex;
    flex-direction: column;
    align-items: center;
  }
  .back {
    align-self: flex-start;
    height: 30px;
    padding: 0 10px 0 6px;
  }
  .choose {
    width: 100%;
    max-width: 860px;
    margin-top: 24px;
    display: flex;
    flex-direction: column;
    gap: 26px;
  }
  .intro {
    text-align: center;
  }
  h1 {
    font-size: var(--text-2xl);
    font-weight: 600;
    text-wrap: balance;
  }
  .intro p,
  .panel-head p {
    margin-top: 6px;
    color: var(--text-dim);
  }
  .big-row {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 14px;
  }
  .big {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    gap: 12px;
    padding: 26px 18px 22px;
    border-radius: 16px;
    border: 1.5px solid var(--border);
    background: var(--bg-raised);
    color: inherit;
    font: inherit;
    cursor: pointer;
    transition:
      border-color 0.15s,
      background 0.15s,
      transform 0.15s;
  }
  .big:hover,
  .small:hover {
    border-color: var(--border-strong);
    background: var(--bg-hover);
    transform: translateY(-2px);
  }
  .big.current,
  .small.current {
    border-color: rgba(var(--accent-rgb), 0.6);
  }
  .logo {
    width: 64px;
    height: 64px;
    border-radius: 18px;
    display: grid;
    place-items: center;
    background: #fff;
    color: var(--text-dim);
  }
  .logo.plain {
    background: var(--bg-active);
  }
  .pname {
    display: block;
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .psub {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: var(--text-md);
    color: var(--text-faint);
  }
  .big.current .psub {
    color: var(--success);
  }
  .own-label {
    margin-bottom: 10px;
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  .small-row {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
  }
  .small {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 16px;
    border-radius: 14px;
    border: 1.5px solid var(--border);
    background: var(--bg-raised);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color 0.15s,
      background 0.15s,
      transform 0.15s;
  }
  .small .pname {
    font-size: var(--text-base);
  }
  .st {
    width: 42px;
    height: 42px;
    flex: none;
    border-radius: 12px;
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--text-dim);
  }
  .assure {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 16px;
    border-radius: 14px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    color: var(--text-dim);
    font-size: var(--text-md);
  }
  .assure :global(svg) {
    flex: none;
    color: var(--success);
  }
  .assure span {
    flex: 1;
  }
  .panel {
    width: 100%;
    max-width: 600px;
    margin-top: 20px;
    padding: 32px;
    border-radius: 20px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  .panel-head {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    gap: 12px;
  }
  .big-logo {
    width: 72px;
    height: 72px;
    border-radius: 20px;
  }
  .panel-head h1 {
    font-size: var(--text-xl);
  }
  .steps {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .steps li {
    display: flex;
    gap: 14px;
    align-items: flex-start;
  }
  .num {
    width: 28px;
    height: 28px;
    flex: none;
    border-radius: 50%;
    display: grid;
    place-items: center;
    font-size: var(--text-md);
    font-weight: 700;
    background: var(--bg-active);
    color: var(--text-dim);
  }
  li:first-child:not(.done) .num,
  li.now .num {
    background: var(--accent-fill);
    color: var(--on-accent);
  }
  li.done .num {
    background: rgba(var(--success-rgb), 0.16);
    color: var(--success);
  }
  li.later {
    opacity: 0.5;
  }
  .step-body {
    flex: 1;
    min-width: 0;
    padding-top: 3px;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
  }
  .step-sub {
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .step-sub.warn {
    color: var(--text-faint);
  }
  .waiting {
    align-self: stretch;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    border-radius: 12px;
    background: var(--accent-soft);
    font-size: var(--text-md);
    font-weight: 600;
  }
  .waiting span:nth-child(2) {
    flex: 1;
  }
  .spin {
    display: grid;
    color: var(--accent-text);
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .fallback {
    border-radius: 12px;
    background: var(--bg-hover);
    border: 1px solid var(--border-strong);
    padding: 0 14px;
  }
  .fallback[open] {
    padding-bottom: 14px;
  }
  .fallback summary {
    padding: 12px 0;
    cursor: pointer;
    font-size: var(--text-md);
    font-weight: 600;
  }
  .fallback label {
    display: block;
    margin-bottom: 8px;
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .row {
    display: flex;
    gap: 8px;
  }
  .row input,
  .fallback input,
  .field input {
    flex: 1;
    min-width: 0;
    width: 100%;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .wide {
    height: 40px;
  }
  .btn.on {
    border-color: var(--accent);
    color: var(--accent-text);
  }
  @media (max-width: 760px) {
    .big-row,
    .small-row,
    .two {
      grid-template-columns: 1fr;
    }
  }
</style>
