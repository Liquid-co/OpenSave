package store

// ForgetPeerKeyForTest removes a peer's pinned public key and its
// authentication latch, putting the pairing in the state a version before
// key exchange left it in.
//
// Test-only, and exported for tests in other packages. There is deliberately
// no production path to this: SetPeerPublicKey refuses an empty key and
// UpsertPeer preserves the existing one, because a pinned key must survive
// every presence refresh and the only legitimate way to lose one is to
// unpair. A test modelling an old pairing needs to write the row directly.
func (s *Store) ForgetPeerKeyForTest(peerID string) error {
	_, err := s.db.Exec(`UPDATE peers SET public_key = '', auth_verified_ms = 0 WHERE id = ?`, peerID)
	return err
}
