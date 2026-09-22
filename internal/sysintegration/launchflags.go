package sysintegration

// StartHiddenFlag is the argument the autostart entry launches OpenSave with.
//
// It exists because starting with the OS and starting by hand are different
// requests that used to be indistinguishable. The Run key launched the bare
// executable, exactly as a double-click does, so every boot brought the full
// window up over whatever the person was about to do — for an app whose job
// at boot is to sit in the tray and sync. Someone who ticked "start with
// Windows" asked for the syncing, not the window.
//
// Defined here, next to the code that writes the entry, so the writer and the
// reader cannot drift apart.
const StartHiddenFlag = "--start-hidden"

// QuitFlag asks a running OpenSave to shut down cleanly and exit.
//
// For the installer, which has to get the app out of the way before it can
// replace or delete its files. The alternative is taskkill, and this app is
// the wrong one to kill: it writes a SQLite database and zips save archives,
// and a snapshot interrupted halfway leaves an archive that is not a
// snapshot. Launching the executable with this flag reaches the running
// instance through the single-instance lock, which runs the same shutdown
// the tray's Quit does — the sync stops, in-flight snapshots are waited for,
// the database is closed.
//
// Launched when nothing is running, it exits without starting anything.
const QuitFlag = "--quit"
