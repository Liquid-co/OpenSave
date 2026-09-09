// What protects traffic with one paired device, and how to say it.
//
// Kept out of the component so it can be tested. The wording matters as much
// as the logic here: this is the text that tells someone whether the saves
// they sync over the internet are readable by whoever else holds their room
// code, and a badge that overstates the protection is worse than no badge,
// because it converts a missing safeguard into a false assurance.
//
// The fields come from the peers API; see PeerProtection in
// internal/p2p/payloadseal.go. The condition for "encrypted" is deliberately
// the same one the send path applies, so the two cannot drift apart.

/**
 * @param {object} peer A peer entry from the peers payload.
 * @returns {'sealed'|'pending'|'open'|'direct'}
 */
export function protectionState(peer) {
  // Fall back to the address when the daemon predates these fields, so an
  // older build shows the neutral local-connection wording rather than
  // claiming a relay pairing is encrypted.
  const overRelay = peer?.overRelay ?? peer?.address === 'relay';
  if (!overRelay) return 'direct';
  if (peer?.encrypted) return 'sealed';
  return peer?.hasKey ? 'pending' : 'open';
}

export const PROTECTION_LABELS = {
  sealed: 'Encrypted',
  pending: 'Encrypting shortly',
  open: 'Not encrypted',
  direct: 'Direct connection',
};

export const PROTECTION_ICONS = {
  sealed: '🔒',
  pending: '🔐',
  open: '🔓',
  direct: '🖧',
};

export const PROTECTION_DETAILS = {
  sealed:
    'Saves sent to this device are sealed between the two of you. The relay — and anyone else holding your room code — passes them on without being able to read them.',
  pending:
    'The key is in place. Encryption starts as soon as this device checks in, which usually takes a few seconds.',
  open: 'Saves sent to this device over the internet can be read by anyone holding your room code, including whoever runs the relay. Pairing over a relay did not keep an encryption key in older versions. Unpair and pair these two devices again to fix it.',
  direct:
    'This device is reached straight over your local network, so nothing passes through a relay. Local traffic is not encrypted, so treat an untrusted network accordingly.',
};

/**
 * Summarises the devices paired through a relay, for the room panel.
 * @param {object[]} peers Every paired peer.
 */
export function roomProtection(peers) {
  const relayPeers = (peers ?? []).filter((p) => protectionState(p) !== 'direct');
  const encrypted = relayPeers.filter((p) => protectionState(p) === 'sealed');
  // Only devices with no key are actionable: 'pending' resolves itself, and
  // telling someone to re-pair over a state that clears in seconds would send
  // them through an unpair for nothing.
  const unprotected = relayPeers.filter((p) => protectionState(p) === 'open');
  return { relayPeers, encrypted, unprotected };
}
