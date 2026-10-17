package daemon

// SwitchBranch switches a game to another branch at someone's request, from
// the window or the command line, and notes the branch it left, so that a
// peer still on that branch is asked to follow instead of being followed back
// by this device's next sync (syncengine/choices.go, GitHub #85). Following a
// peer's branch during a sync is not this: it goes through the sync engine.
func (d *Daemon) SwitchBranch(gameID, name string) error {
	from := ""
	if game, err := d.Store.GetGame(gameID); err == nil {
		from = game.ActiveBranch
	}
	if err := d.Snapshots.SwitchBranch(gameID, name); err != nil {
		return err
	}
	if from != name && d.P2P != nil && d.P2P.Sync != nil {
		d.P2P.Sync.NoteBranchSwitch(gameID, from)
	}
	return nil
}
