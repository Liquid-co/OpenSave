<script>
  import { t as translateUI } from '../../lib/i18n.js';
  // Signing in to Google Drive, OneDrive or Dropbox — or the account card
  // once signed in. Kept mounted while other cards are looked at, so a
  // sign-in finishing in the browser meanwhile is still caught.
  import { onDestroy } from 'svelte';
  import { toast, cloudAuthEvent } from '../../lib/stores.js';
  import { api, native } from '../../lib/api.js';
  import { isOAuth, isEmail, providerById } from '../../lib/cloudproviders.js';
  import ProviderIcon from './ProviderIcon.svelte';
  import Spinner from '../../components/ui/Spinner.svelte';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';

  export let config;
  export let connected = null;
  /** The page's busy flag (a writable store). */
  export let busy;
  /** Re-reads the settings after the sign-in changes. */
  export let reload = async () => {};

  let authCode = '';
  let authInProgress = false;
  let authAuto = false; // backend caught the redirect automatically

  $: current = providerById(config.provider);

  // The daemon broadcasts cloud-auth when the browser redirect lands on
  // its temporary localhost listener — sign-in completes with no pasting.
  const unsubAuth = cloudAuthEvent.subscribe((ev) => {
    if (!ev || !authInProgress) return;
    cloudAuthEvent.set(null);
    authInProgress = false;
    if (ev.success) {
      toast($translateUI('oAuthConnect.connectedAs', { p0: ev.userEmail }), 'success');
      reload();
    } else {
      toast(ev.error ?? $translateUI('oAuthConnect.signInFailed'), 'error');
    }
  });
  onDestroy(unsubAuth);

  async function startAuth() {
    busy.set(true);
    try {
      const res = await api.post('/api/auth/start', { provider: config.provider });
      native.openExternal(res.authUrl);
      authInProgress = true;
      authAuto = !!res.autoCallback;
      if (authAuto) {
        toast($translateUI('oAuthConnect.finishSigningInOpenSaveConnectsAutomatically'));
      } else {
        toast($translateUI('oAuthConnect.signInUsingTheBrowserWindowThen'));
      }
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy.set(false);
    }
  }

  function cancelAuth() {
    authInProgress = false;
    authCode = '';
  }

  async function finishAuth() {
    busy.set(true);
    try {
      const res = await api.post('/api/auth/callback', { code: authCode.trim() });
      toast($translateUI('oAuthConnect.connectedAs', { p0: res.userEmail }), 'success');
      authInProgress = false;
      authCode = '';
      await reload();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy.set(false);
    }
  }

  async function disconnect() {
    busy.set(true);
    try {
      await api.post('/api/auth/disconnect');
      toast($translateUI('oAuthConnect.disconnected'));
      await reload();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy.set(false);
    }
  }
</script>

{#if isOAuth(config.provider)}
  {#if connected === config.provider && config.tokens?.userEmail}
    <div class="acct">
      <div class="acct-icon"><ProviderIcon provider={current} size={26} /></div>
      <div class="acct-info">
        <div class="acct-title">{$translateUI('oAuthConnect.connectedTo')} {current?.label}</div>
        <div class="acct-sub">
          <span class="acct-dot"></span>
          {isEmail(config.tokens.userEmail) ? config.tokens.userEmail : $translateUI('oAuthConnect.signedInNewSnapshotsUploadAutomatically')}
        </div>
      </div>
      <button class="btn small danger" disabled={$busy} on:click={disconnect}>{$translateUI('oAuthConnect.disconnect')}</button>
    </div>
  {:else}
    {#if connected}
      <p class="quiet" style="margin-bottom: 10px;">
        {$translateUI('oAuthConnect.youReCurrentlyConnectedTo')} {providerById(connected)?.label} {$translateUI('oAuthConnect.signingInHereWillReplaceThat')}
      </p>
    {/if}
    {#if !authInProgress}
      <button class="btn primary" disabled={$busy} on:click={startAuth}>
        {$translateUI('oAuthConnect.signInWith')} {current?.label}
      </button>
      {#if config.provider === 'google_drive'}
        <p class="quiet" style="margin-top: 10px;">
          <TriangleAlert size={14} class="inline-icon warn-icon" />{$translateUI('oAuthConnect.onGoogleSConsentScreen')} <strong>{$translateUI('oAuthConnect.tickTheCheckbox')}</strong> {$translateUI('oAuthConnect.allowingOpenSaveToAccessItsOwnDrive')}
        </p>
      {/if}
    {:else}
      <div class="waiting">
        <Spinner size={16} />
        <div class="waiting-text">
          <strong>{$translateUI('oAuthConnect.waitingForYouToFinishSigningIn')}</strong>
          <span class="quiet">
            {authAuto ? $translateUI('oAuthConnect.approveAccessInYourBrowserOpenSave') : $translateUI('oAuthConnect.approveAccessInYourBrowserThenPaste')}
          </span>
        </div>
        <button class="btn small" on:click={cancelAuth}>{$translateUI('common.cancel')}</button>
      </div>
      <!-- Always shown, not behind a link: when the browser can't reach
           OpenSave it shows an error page with the code in its address bar,
           and this is the only way on from there (GitHub #39). -->
      <div class="code">
        <p class="quiet">
          {authAuto ? $translateUI('oAuthConnect.ifYourBrowserShowsAnErrorPage') : $translateUI('oAuthConnect.afterApprovingAccessTheBrowserLandsOn')}{$translateUI('oAuthConnect.copyTheWholeAddressFromItsAddress')}
        </p>
        <div class="path-row">
          <input placeholder="http://localhost/callback?code=…" bind:value={authCode} aria-label="{$translateUI('oAuthConnect.addressOrCodeFromTheBrowser')}" />
          <button class="btn primary" disabled={!authCode.trim() || $busy} on:click={finishAuth}>{$translateUI('oAuthConnect.connect')}</button>
        </div>
      </div>
    {/if}
  {/if}
{/if}

<style>
  .quiet {
    color: var(--text-faint);
    font-size: 0.85rem;
  }
  .acct {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 16px 18px;
    background: rgba(var(--success-rgb), 0.05);
    border: 1px solid rgba(var(--success-rgb), 0.3);
    border-radius: var(--radius-lg);
  }
  .acct-icon {
    width: 44px;
    height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 11px;
    color: var(--text-dim);
    flex-shrink: 0;
  }
  .acct-info {
    flex: 1;
    min-width: 0;
  }
  .acct-title {
    font-weight: 600;
    font-size: 0.98rem;
  }
  .acct-sub {
    display: flex;
    align-items: center;
    gap: 7px;
    color: var(--text-dim);
    font-size: 0.82rem;
    margin-top: 3px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .acct-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--success);
    flex-shrink: 0;
  }
  .waiting {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 14px 16px;
    background: var(--accent-soft);
    border: 1px solid var(--accent);
    border-radius: var(--radius);
  }
  .waiting-text {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 0.88rem;
  }
  .code {
    margin-top: 12px;
  }
</style>
