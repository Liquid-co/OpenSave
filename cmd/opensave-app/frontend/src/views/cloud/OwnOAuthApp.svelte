<script>
  // ── Your own OAuth app ───────────────────────────────────────────
  // The daemon has always supported a per-provider client id and secret;
  // nothing in the app set them, so the only route was a hand-written API
  // call. Two things needed it. Google's built-in credentials are a shared
  // app that can expire weekly, and OneDrive has NO built-in id at all —
  // Microsoft does not allow a shared public one — so the OneDrive card could
  // be selected but never connect, and the error said to configure it on a
  // screen that did not exist.
  //
  // OneDrive therefore shows these fields open, as its normal setup rather
  // than as a workaround; the others keep them folded away.
  import { toast, settings } from '../../lib/stores.js';
  import { api, native } from '../../lib/api.js';
  import { needsOwnApp } from '../../lib/cloudproviders.js';

  export let config;
  export let connected = null;
  export let busy;
  /** Re-reads the settings, keeping the given provider selected. */
  export let reload = async (keepProvider) => {};

  // Mounted per provider (the page rebuilds this when the card changes), so
  // one provider's credentials never show under another's name.
  const provider = config.provider;
  let ownAppID = config.customClientIds?.[provider] ?? '';
  let ownAppSecret = config.customClientSecrets?.[provider] ?? '';
  let showOwnApp = needsOwnApp(provider) && !ownAppID;

  async function save() {
    // Changing the id invalidates any tokens already held: they were issued to
    // the old app and no refresh will be accepted. Disconnecting is the honest
    // outcome — leaving them in place would show "Connected" over credentials
    // that cannot work.
    const changed = (config.customClientIds?.[provider] ?? '') !== ownAppID.trim();
    const wasConnected = connected === provider;

    busy.set(true);
    try {
      settings.set(
        await api.post('/api/settings', {
          cloudSync: {
            customClientIds: { [provider]: ownAppID.trim() },
            customClientSecrets: { [provider]: ownAppSecret.trim() }
          }
        })
      );
      if (changed && wasConnected) {
        await api.post('/api/auth/disconnect');
        toast('Saved. Sign in again — the old connection belonged to the previous app.', 'success');
      } else {
        toast(ownAppID.trim() ? 'Saved. Sign in to use your own app.' : 'Cleared — the built-in app will be used.', 'success');
      }
      // Reload for the fresh tokens/ids, but keep this card selected. The
      // stored provider is whatever was last saved with "Save settings" — so
      // without that, saving credentials for a provider you had only selected
      // drops you back onto the stored one, and the section you were filling
      // in disappears as though nothing happened.
      await reload(provider);
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy.set(false);
    }
  }
</script>

<div class="own-app">
  {#if needsOwnApp(provider) && !ownAppID}
    <p class="quiet required">
      <strong>OneDrive needs your own app registration.</strong> Microsoft doesn't allow a
      shared one, so OpenSave can't ship credentials for it. Create a free app in the
      <button class="linkish" on:click={() => native.openExternal('https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade')}>Azure portal</button>
      with redirect URI <code>http://localhost/callback</code>, then paste its Application
      (client) ID below.
    </p>
  {:else}
    <button class="linkish" on:click={() => (showOwnApp = !showOwnApp)}>
      {showOwnApp ? '▾' : '▸'} Use your own OAuth app{ownAppID ? ' (in use)' : ''}
    </button>
  {/if}

  {#if showOwnApp || (needsOwnApp(provider) && !ownAppID)}
    <div class="body">
      {#if provider === 'google_drive'}
        <p class="quiet">
          OpenSave's built-in Google credentials are a shared app still in testing, so Drive
          can ask you to sign in again every week. Your own Client ID stops that. Create one
          in the
          <button class="linkish" on:click={() => native.openExternal('https://console.cloud.google.com/apis/credentials')}>Google Cloud console</button>
          as an OAuth client with redirect URI <code>http://localhost/callback</code>.
        </p>
      {:else if provider === 'dropbox'}
        <p class="quiet">
          Only needed if you'd rather use your own Dropbox app than the built-in one.
        </p>
      {/if}
      <div class="field">
        <label for="cb-clientid">Client ID</label>
        <input
          id="cb-clientid"
          bind:value={ownAppID}
          spellcheck="false"
          placeholder={provider === 'google_drive' ? '…apps.googleusercontent.com' : 'Application (client) ID'}
        />
      </div>
      <div class="field">
        <label for="cb-clientsecret">Client secret <span class="quiet">— leave empty unless your app requires one</span></label>
        <input id="cb-clientsecret" type="password" bind:value={ownAppSecret} spellcheck="false" />
      </div>
      <div class="path-row">
        <button class="btn primary" disabled={$busy} on:click={save}>Save credentials</button>
        {#if ownAppID}
          <button
            class="btn small"
            disabled={$busy}
            title="Go back to OpenSave's built-in credentials"
            on:click={() => { ownAppID = ''; ownAppSecret = ''; save(); }}
          >
            Use the built-in app
          </button>
        {/if}
      </div>
      {#if connected === provider}
        <p class="quiet">
          You're signed in already — changing this disconnects you, because the existing
          sign-in belongs to the old app.
        </p>
      {/if}
    </div>
  {/if}
</div>

<style>
  .quiet {
    color: var(--text-faint);
    font-size: 0.85rem;
  }
  .own-app {
    margin-top: 16px;
    padding-top: 14px;
    border-top: 1px solid var(--border);
  }
  .required {
    margin: 0 0 10px;
  }
  .body {
    margin-top: 10px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .linkish {
    margin-top: 10px;
    border: none;
    background: transparent;
    color: var(--text-faint);
    font-size: 0.8rem;
    cursor: pointer;
    text-decoration: underline;
    padding: 2px 0;
  }
  .linkish:hover {
    color: var(--text-dim);
  }
</style>
