import { writable } from 'svelte/store';

// Bumped once after every successful movement create, regardless of which
// entry point created it (the home route's inline AddMovementForm, or the
// layout-level QuickAdd reachable from anywhere — see FRONT-09). Anything
// that lists or caches movements subscribes and refreshes itself, instead
// of the creator having to know who else needs telling. This is what lets
// QuickAdd (mounted once in +layout.svelte, so it outlives any single
// route) and the home route's movement list stay in sync without prop
// drilling a callback through the layout.
export const movementCreated = writable(0);

export function notifyMovementCreated() {
	movementCreated.update((n) => n + 1);
}
