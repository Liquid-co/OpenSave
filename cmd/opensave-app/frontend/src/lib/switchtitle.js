// A Nintendo Switch game's title id, from the save folder its emulator keeps
// it in: …/save/<account>/<profile>/<title id>. The same rule as the daemon's
// internal/switchtitle, which is what the cover for it is asked by — the
// daemon makes it from the icon the emulator keeps (api/cover_switch.go).

const TITLE_ID = /^[0-9a-f]{16}$/i;

/** The title id of a Switch save folder, upper case, or '' for any other. */
export function switchTitleId(savePath) {
  const parts = String(savePath ?? '')
    .split(/[\\/]+/)
    .filter(Boolean);
  if (parts.length < 4) return '';
  const id = parts[parts.length - 1];
  if (!TITLE_ID.test(id) || parts[parts.length - 4].toLowerCase() !== 'save') return '';
  return id.toUpperCase();
}
