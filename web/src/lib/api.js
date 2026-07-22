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

export function getCurrencies() {
	return request('/currencies');
}

export function addCurrency(code) {
	return request('/currencies', {
		method: 'POST',
		body: JSON.stringify({ code })
	});
}

export function listAccounts() {
	return request('/accounts');
}

export function createAccount({ name, type, currency }) {
	return request('/accounts', {
		method: 'POST',
		body: JSON.stringify({ name, type, currency })
	});
}

// balance is in the smallest currency unit, like movement amounts.
export function reportAccountBalance(id, balance) {
	return request(`/accounts/${id}/balance`, {
		method: 'POST',
		body: JSON.stringify({ balance })
	});
}

// from/to as YYYY-MM-DD; to is inclusive.
export function getCashflow(from, to) {
	return request(`/cashflow?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`);
}

export function createMovement({
	amount,
	currency,
	description,
	category,
	payment_method,
	installments,
	account_id
}) {
	return request('/movements', {
		method: 'POST',
		body: JSON.stringify({
			amount,
			currency,
			description,
			category,
			payment_method,
			installments,
			account_id
		})
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
