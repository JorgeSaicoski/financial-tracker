import { writable } from 'svelte/store';

// The sidebar (mounted once in +layout.svelte, reachable from any route)
// and the home route's inline AddMovementForm need shared state without
// prop drilling through the layout — same problem/solution shape as
// movementEvents.js. id increments on every trigger so a subscriber can
// tell "a new shortcut was clicked" apart from its own initial subscribe
// callback (a writable store fires synchronously with its current value
// on subscribe).
export const quickAction = writable({ id: 0, direction: '', category: '' });

export function triggerQuickAction(direction, category) {
	quickAction.update((s) => ({ id: s.id + 1, direction, category }));
}
