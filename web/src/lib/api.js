import { env } from '$env/dynamic/public';

const BASE_URL = env.PUBLIC_API_URL || 'http://localhost:8081';

async function request(path, options = {}) {
	const res = await fetch(`${BASE_URL}${path}`, {
		headers: { 'Content-Type': 'application/json' },
		...options
	});

	const body = await res.json().catch(() => null);

	if (!res.ok) {
		throw new Error(body?.error ?? `request failed with status ${res.status}`);
	}

	return body;
}

export function listMovements() {
	return request('/movements');
}

export function getCategories() {
	return request('/categories');
}

export function createMovement({ amount, currency, description, category, payment_method, installments }) {
	return request('/movements', {
		method: 'POST',
		body: JSON.stringify({ amount, currency, description, category, payment_method, installments })
	});
}

export function cancelMovement(id) {
	return request(`/movements/${id}/cancel`, { method: 'POST' });
}

export function cancelCreditCardPurchase(id) {
	return request(`/credit-card-purchases/${id}/cancel`, { method: 'POST' });
}

export function syncNow() {
	return request('/sync', { method: 'POST' });
}
