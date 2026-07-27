import { writable } from 'svelte/store';
import { listCards, createCard, updateCard, deleteCard } from '$lib/api.js';

// Cards are read by both the sidebar (registration/management, mounted
// once in +layout.svelte) and the home route (card picker on
// AddMovementForm, card chip on MovementRow) — a shared store avoids
// prop-drilling the list through +layout.svelte, which doesn't otherwise
// fetch any data itself. Callers that mutate (add/edit/remove) update
// this store from the API response directly rather than refetching the
// whole list.
export const cards = writable([]);

export async function loadCards() {
	const data = await listCards();
	cards.set(data.cards ?? []);
}

export async function addCard(input) {
	const created = await createCard(input);
	cards.update((list) => [...list, created]);
	return created;
}

export async function editCard(id, patch) {
	const updated = await updateCard(id, patch);
	cards.update((list) => list.map((c) => (c.id === id ? updated : c)));
	return updated;
}

export async function removeCard(id) {
	await deleteCard(id);
	cards.update((list) => list.filter((c) => c.id !== id));
}
