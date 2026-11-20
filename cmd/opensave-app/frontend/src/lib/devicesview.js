// What the Devices page says about each paired device, as plain functions of
// the stores' values so each can be tested.

/** How a paired device is reached now: on this network, over the internet
 *  (through the relay), or not at all. */
export function peerWhere(peer) {
  if (peer?.status !== 'online') return 'offline';
  return peer.address === 'relay' || peer.overRelay ? 'internet' : 'network';
}

/** The games this device and a paired one both have: the ones synced with
 *  it at least once, by name. */
export function gamesOnBoth(games, peerId) {
  return Object.values(games ?? {})
    .filter((g) => g.lastSyncedWith && peerId in g.lastSyncedWith)
    .sort((a, b) => a.name.localeCompare(b.name));
}

/** The latest files sent to or received from a device, newest first, from
 *  the activity report's items. */
export function recentWith(items, deviceName, limit = 2) {
  return (items ?? [])
    .filter((i) => (i.kind === 'sent' || i.kind === 'received') && i.device === deviceName)
    .sort((a, b) => b.atMs - a.atMs)
    .slice(0, limit);
}

const parts = (v) =>
  String(v ?? '')
    .replace(/^v/, '')
    .split(/[.-]/)
    .map((p) => (/^\d+$/.test(p) ? Number(p) : p));

/** Whether a paired device runs an older OpenSave than this one: by number,
 *  and a pre-release (2.5.0-beta.1) before its release (2.5.0). Unknown
 *  versions are not called older. */
export function isOlder(peerVersion, myVersion) {
  if (!peerVersion || !myVersion || /dev/i.test(myVersion)) return false;
  const a = parts(peerVersion);
  const b = parts(myVersion);
  for (let i = 0; i < 3; i++) {
    const x = Number(a[i] ?? 0);
    const y = Number(b[i] ?? 0);
    if (x !== y) return x < y;
  }
  const aPre = a.length > 3;
  const bPre = b.length > 3;
  return aPre && !bPre;
}

// Home routers hand out 192.168 and 10 addresses; 172.16-31 is as often a
// virtual adapter (WSL, Docker) that no other device can reach, and 100.64
// is Tailscale's. In that order.
const HOME_RANGES = [/^192\.168\./, /^10\./, /^100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\./, /^172\.(1[6-9]|2\d|3[01])\./];

/** The address other devices at home reach a relay hosted here at: the
 *  likeliest home address, ws:// on the relay's port. */
export function homeRelayAddress(lanIps, port) {
  const list = lanIps ?? [];
  let ip = '';
  for (const range of HOME_RANGES) {
    ip = list.find((a) => range.test(a)) ?? '';
    if (ip) break;
  }
  ip = ip || list[0] || '';
  return ip && port ? `ws://${ip}:${port}` : '';
}
