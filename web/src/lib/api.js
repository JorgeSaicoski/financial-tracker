import { env } from '$env/dynamic/public';
import { getAccessToken, handleUnauthorized } from './auth.svelte.js';

const BASE_URL = env.PUBLIC_API_URL || 'http://localhost:8081';

async function request(path, options = {}) {
	const headers = { 'Content-Type': 'application/json', ...options.headers };
	const token = getAccessToken();
	if (token) {
		headers['Authorization'] = `Bearer ${token}`;
	}

	const res = await fetch(`${BASE_URL}${path}`, {
		...options,
		headers
	});

	if (res.status === 401) {
		handleUnauthorized();
		throw new Error('session expired — please log in again');
	}

	const body = await res.json().catch(() => null);

	if (!res.ok) {
		throw new Error(body?.error ?? `request failed with status ${res.status}`);
	}

	return body;
}

// --- Movements ---

// params is an optional subset of the GET /movements query params
// (currency, from, to, limit, offset). Omitted/empty values aren't sent,
// so listMovements() with no args behaves exactly as before.
export function listMovements(params = {}) {
	const query = new URLSearchParams();
	for (const [key, value] of Object.entries(params)) {
		if (value !== undefined && value !== null && value !== '') {
			query.set(key, value);
		}
	}
	const qs = query.toString();
	return request(`/movements${qs ? `?${qs}` : ''}`);
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

// --- Import ---

export function getImportSpec() {
	return request('/import/movements/spec');
}

// csvText is the raw CSV body (spec's fixed header + data rows). Options
// map straight onto the endpoint's query params; all default to false.
export function importMovements(csvText, { dryRun = false, allowPartial = false, skipDuplicates = false } = {}) {
	const query = new URLSearchParams();
	if (dryRun) query.set('dry_run', 'true');
	if (allowPartial) query.set('allow_partial', 'true');
	if (skipDuplicates) query.set('skip_duplicates', 'true');
	const qs = query.toString();
	return request(`/import/movements${qs ? `?${qs}` : ''}`, {
		method: 'POST',
		headers: { 'Content-Type': 'text/csv' },
		body: csvText
	});
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

// --- Local archive (BACK-15) ---

export function getLocalArchiveSetting() {
	return request('/settings/local-archive');
}

export function setLocalArchiveSetting(enabled) {
	return request('/settings/local-archive', {
		method: 'PUT',
		body: JSON.stringify({ local_archive_enabled: enabled })
	});
}

// Plaintext account export — the caller (LocalBackupPanel) encrypts this
// client-side before it's saved anywhere.
export function exportArchive() {
	return request('/export/archive');
}

// bundle is an already-decrypted archive (same shape exportArchive()
// returns): { accounts, movements, credit_card_purchases }.
export function importArchive(bundle) {
	return request('/import/archive', {
		method: 'POST',
		body: JSON.stringify(bundle)
	});
}
