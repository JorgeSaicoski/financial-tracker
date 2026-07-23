// Shared formatting/label helpers used across the movements list, the edit
// form and the transfer form, so the three don't redefine the same lookups.

export const categoryIcons = {
	food: '🍽️',
	transport: '🚌',
	housing: '🏠',
	utilities: '💡',
	health: '🏥',
	entertainment: '🎬',
	shopping: '🛍️',
	education: '📚',
	income: '💰',
	transfer: '🔁',
	other: '📦'
};

export const paymentMethodLabels = {
	cash: 'Cash',
	debit_card: 'Debit card',
	credit_card: 'Credit card',
	pix: 'Pix',
	bank_transfer: 'Bank transfer',
	other: 'Other'
};

export const accountTypeIcons = {
	bank: '🏦',
	investment: '📈',
	crypto: '🪙',
	cash: '💵',
	other: '📦'
};

export function formatAmount(cents, currency) {
	// Intl only knows ISO 4217 codes; btc & friends fall back to a plain
	// "0.00 BTC" rendering.
	try {
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: currency.toUpperCase()
		}).format(cents / 100);
	} catch {
		return `${(cents / 100).toFixed(2)} ${currency.toUpperCase()}`;
	}
}

export function labelFor(value) {
	return value.charAt(0).toUpperCase() + value.slice(1);
}

// rowState reflects the fields the API returns for a movement into one of
// the states the UI cares about for enabling/disabling actions.
export function rowState(movement) {
	if (movement.status === 'voided') return 'voided';
	if (movement.cancels_movement_id) return 'reversal';
	if (movement.reversed_by_movement_id) return 'reversed';
	return 'active';
}

export function localDate(d) {
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}
