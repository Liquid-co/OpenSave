// Deciding whether a save-folder edit is a real move.
//
// The game PATCH decodes into the stored game, so a field that is present is
// an edit and a field that is absent leaves the stored value alone. That makes
// "should this be sent at all" a real decision rather than a formality:
// sending the folder unchanged is harmless, but sending an empty string is an
// edit to nothing, and validation rejects it — which would block every other
// change on the same form.

/**
 * The folder to send in a game PATCH, or null when nothing should be sent.
 *
 * @param {string|undefined|null} currentPath the folder the game is tracked at
 * @param {string|undefined|null} editedPath  what the user left in the box
 * @returns {string|null} the new folder, or null to leave it untouched
 */
export function savePathChange(currentPath, editedPath) {
  const next = String(editedPath ?? '').trim();
  // Blank means the user cleared the box, not that they want no save folder —
  // a game must always have one, so this leaves the current folder in place.
  if (next === '') return null;
  if (next === String(currentPath ?? '')) return null;
  return next;
}
