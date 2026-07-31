import { writable } from 'svelte/store';

// Bumped once after a new account is created. Sidebar.svelte fetches its
// own accounts independently (for the "Pay card" account picker — it's
// mounted once in +layout.svelte, outliving any single route, so it
// can't rely on the home route's accounts state) and has no other way to
// learn a new account showed up. Same shape as movementEvents.js.
export const accountsChanged = writable(0);

export function notifyAccountsChanged() {
	accountsChanged.update((n) => n + 1);
}
