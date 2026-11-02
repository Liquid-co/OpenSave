<script>
  import { t as translateUI } from '../../lib/i18n.js';
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
  import Chevron from '../../components/ui/Chevron.svelte';

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
        toast($translateUI('ownOAuthApp.savedSignInAgainTheOld'), 'success');
      } else {
        toast(ownAppID.trim() ? $translateUI('ownOAuthApp.savedSignInToUseYourOwn') : $translateUI('ownOAuthApp.clearedTheBuiltInAppWillBe'), 'success');
      }
      // Reload for the fresh tokens/ids, but keep this card selected. The
      // stored provider is whatever was last saved with "Save provider" — so
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
      <strong>{$translateUI('ownOAuthApp.oneDriveNeedsYourOwnAppRegistration')}</strong> {$translateUI('ownOAuthApp.microsoftDoesnTAllowASharedOneSo')}
      <button class="linkish" on:click={() => native.openExternal('https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade')}>{$translateUI('ownOAuthApp.azurePortal')}</button>
      {$translateUI('ownOAuthApp.withRedirectURI')} <code>http://localhost/callback</code>{$translateUI('ownOAuthApp.thenPasteItsApplicationClientID')}
    </p>
  {:else}
    <button class="linkish" on:click={() => (showOwnApp = !showOwnApp)}>
      <Chevron open={showOwnApp} size={12} /> {$translateUI('ownOAuthApp.useYourOwnOAuthApp')}{ownAppID ? $translateUI('ownOAuthApp.inUse') : ''}
    </button>
  {/if}

  {#if showOwnApp || (needsOwnApp(provider) && !ownAppID)}
    <div class="body">
      {#if provider === 'google_drive'}
        <p class="quiet">
          {$translateUI('ownOAuthApp.openSaveSBuiltInGoogleCredentialsAreAShared')}
          <button class="linkish" on:click={() => native.openExternal('https://console.cloud.google.com/apis/credentials')}>{$translateUI('ownOAuthApp.googleCloudConsole')}</button>
          {$translateUI('ownOAuthApp.asAnOAuthClientWithRedirectURI')} <code>http://localhost/callback</code>.
        </p>
      {:else if provider === 'dropbox'}
        <p class="quiet">
          {$translateUI('ownOAuthApp.onlyNeededIfYouDRatherUseYour')}
        </p>
      {/if}
      <div class="field">
        <label for="cb-clientid">{$translateUI('ownOAuthApp.clientID')}</label>
        <input
          id="cb-clientid"
          bind:value={ownAppID}
          spellcheck="false"
          placeholder={provider === 'google_drive' ? '…apps.googleusercontent.com' : $translateUI('ownOAuthApp.applicationClientID')}
        />
      </div>
      <div class="field">
        <label for="cb-clientsecret">{$translateUI('ownOAuthApp.clientSecret')} <span class="quiet">{$translateUI('ownOAuthApp.leaveEmptyUnlessYourAppRequires')}</span></label>
        <input id="cb-clientsecret" type="password" bind:value={ownAppSecret} spellcheck="false" />
      </div>
      <div class="path-row">
        <button class="btn primary" disabled={$busy} on:click={save}>{$translateUI('ownOAuthApp.saveCredentials')}</button>
        {#if ownAppID}
          <button
            class="btn small"
            disabled={$busy}
            title="{$translateUI('ownOAuthApp.goBackToOpenSaveSBuiltInCredentials')}"
            on:click={() => { ownAppID = ''; ownAppSecret = ''; save(); }}
          >
            {$translateUI('ownOAuthApp.useTheBuiltInApp')}
          </button>
        {/if}
      </div>
      {#if connected === provider}
        <p class="quiet">
          {$translateUI('ownOAuthApp.youReSignedInAlreadyChangingThis')}
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
