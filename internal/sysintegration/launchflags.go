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
