-- Whether a game's device settings (the files the Ludusavi manifest tags as
-- its settings and not its save, presets/devicesettings.go) sync like the rest
-- of its save. Off by default: one PC's resolution and graphics quality are
-- not another's.
ALTER TABLE games ADD COLUMN sync_device_settings INTEGER NOT NULL DEFAULT 0;
