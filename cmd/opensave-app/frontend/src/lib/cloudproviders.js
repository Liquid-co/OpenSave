// The cloud backup providers and the rules for what each card says.

export const providers = [
  { id: 'google_drive', label: 'Google Drive', oauth: true, img: 'cloud/googledrive.png' },
  { id: 'onedrive', label: 'OneDrive', oauth: true, img: 'cloud/onedrive.png' },
  { id: 'dropbox', label: 'Dropbox', oauth: true, img: 'cloud/dropbox.png' },
  { id: 'local', label: 'Local Folder', oauth: false, icon: 'folder' },
  { id: 'webdav', label: 'WebDAV Server', oauth: false, icon: 'cloud' },
  { id: 'webhook', label: 'HTTP Webhook', oauth: false, icon: 'webhook' }
];

export const providerById = (id) => providers.find((p) => p.id === id);

const oauthIds = new Set(providers.filter((p) => p.oauth).map((p) => p.id));
export const isOAuth = (id) => oauthIds.has(id);

// Some providers can't reveal the account email (privacy settings / missing
// scope) — the daemon stores a placeholder then. Only display it if it
// actually looks like an address.
export const isEmail = (s) => /\S+@\S+\.\S+/.test(s ?? '');

// OneDrive has NO built-in app: Microsoft does not allow a shared public one,
// so it needs the person's own registration as its normal setup.
export const needsOwnApp = (id) => id === 'onedrive';

/** Which provider the stored sign-in belongs to, if any. OAuth tokens survive
 *  a provider switch (so switching back reconnects instantly), but only the
 *  OAuth provider they were issued for is connected — never badge a local
 *  folder or WebDAV card off someone's Drive tokens. */
export function connectedProviderOf(cfg) {
  return cfg?.tokens?.userEmail && isOAuth(cfg.provider) ? cfg.provider : null;
}

/** The line under a provider's card. */
export function providerStatus(id, connected, cfg) {
  if (!cfg) return '';
  if (id === connected) {
    const email = cfg.tokens?.userEmail;
    return isEmail(email) ? email : 'Connected';
  }
  if (isOAuth(id)) return 'Click to sign in';
  if (id === cfg.provider && cfg.url) return 'Configured';
  return 'Not configured';
}

/** Tracked games and games only in the cloud, one tile each, so the cloud
 *  browser can both show what is up there and upload what isn't yet. */
export function cloudTiles(cloudGames, localGames) {
  const inCloud = new Set((cloudGames ?? []).map((g) => g.gameId));
  return [
    ...(cloudGames ?? []).map((g) => {
      const local = localGames.find((x) => x.id === g.gameId);
      return { id: g.gameId, name: g.gameName, coverUrl: local?.coverUrl, tracked: !!local, cloud: g };
    }),
    ...localGames
      .filter((lg) => !inCloud.has(lg.id))
      .map((lg) => ({ id: lg.id, name: lg.name, coverUrl: lg.coverUrl, tracked: true, cloud: null }))
  ];
}
