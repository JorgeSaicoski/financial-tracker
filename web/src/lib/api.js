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

// --- Movements ---

export function listMovements() {
	return request('/movements');
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

// patch is a partial UpdateMovementRequest body: only include the fields
// that should change. account_id: '' explicitly clears the account.
export function updateMovement(id, patch) {
	return request(`/movements/${id}`, {
		method: 'PATCH',
		body: JSON.stringify(patch)
	});
}

export function cancelMovement(id) {
	return request(`/movements/${id}/cancel`, { method: 'POST' });
}

export function cancelCreditCardPurchase(id) {
	return request(`/credit-card-purchases/${id}/cancel`, { method: 'POST' });
}

export function getCategories() {
	return request('/categories');
}

// from/to as YYYY-MM-DD; to is inclusive.
export function getCashflow(from, to) {
	return request(`/cashflow?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`);
}

export function syncNow() {
	return request('/sync', { method: 'POST' });
}

// --- Accounts ---

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

// --- Transfers ---

// from_account_id/to_account_id must hold the same currency (v1); amount is
// positive in that shared currency, timestamp is an ISO string.
export function createTransfer({ from_account_id, to_account_id, amount, description, timestamp }) {
	return request('/transfers', {
		method: 'POST',
		body: JSON.stringify({ from_account_id, to_account_id, amount, description, timestamp })
	});
}

export function cancelTransfer(id) {
	return request(`/transfers/${id}/cancel`, { method: 'POST' });
}

// --- Currencies ---

export function getCurrencies() {
	return request('/currencies');
}

export function addCurrency(code) {
	return request('/currencies', {
		method: 'POST',
		body: JSON.stringify({ code })
	});
}
