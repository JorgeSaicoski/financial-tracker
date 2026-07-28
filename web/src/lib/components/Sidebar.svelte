<script>
	import { onDestroy, onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { listAccounts, getCurrencies, createMovement } from '$lib/api.js';
	import { cards, loadCards, addCard, editCard, removeCard } from '$lib/stores/cards.js';
	import { movementCreated, notifyMovementCreated } from '$lib/stores/movementEvents.js';
	import { accountsChanged } from '$lib/stores/accountEvents.js';
	import { triggerQuickAction } from '$lib/stores/quickAction.js';
	import { formatAmount } from '$lib/format.js';
	import CardForm from './CardForm.svelte';

	// Quick-add shortcuts prefill AddMovementForm on `/` via the
	// quickAction store (FRONT-07) — icons here intentionally match
	// format.js's categoryIcons for the same category, so the shortcut and
	// the form's own category select read as the same thing.
	const quickActions = [
		{ label: 'Income', icon: '💰', direction: 'income', category: 'income' },
		{ label: 'Market', icon: '🍽️', direction: 'expense', category: 'food' },
		{ label: 'Shop', icon: '🛍️', direction: 'expense', category: 'shopping' },
		{ label: 'Bill', icon: '💡', direction: 'expense', category: 'utilities' }
	];

	function handleQuickAction(action) {
		triggerQuickAction(action.direction, action.category);
		if ($page.url.pathname !== '/') {
			goto('/');
		}
	}

	// Accounts/currencies here are the sidebar's own read-only fetch — it
	// mounts once in +layout.svelte, outliving any single route, so it
	// can't rely on the home route's own accounts state. Only used to
	// populate the "Pay card" account picker.
	let accounts = $state([]);
	let currencies = $state(['usd', 'brl']);
	let cardsError = $state('');

	let addingCard = $state(false);
	let editingCardId = $state(null);
	let payingCardId = $state(null);
	let payAccountId = $state('');
	let payAmountInput = $state('');
	let paySubmitting = $state(false);
	let payError = $state('');

	async function refreshAccounts() {
		try {
			const data = await listAccounts();
			accounts = data.accounts ?? [];
		} catch {
			// The rest of the sidebar still works without accounts; only
			// "Pay card" needs them.
		}
	}

	async function refreshCurrencies() {
		try {
			const data = await getCurrencies();
			if (data.currencies?.length) currencies = data.currencies;
		} catch {
			// Keep the usd/brl defaults.
		}
	}

	async function refreshCards() {
		cardsError = '';
		try {
			await loadCards();
		} catch (err) {
			cardsError = err.message;
		}
	}

	async function handleAddCard(payload) {
		await addCard(payload);
		addingCard = false;
	}

	async function handleEditCard(id, payload) {
		await editCard(id, payload);
		editingCardId = null;
	}

	async function handleDeleteCard(card) {
		if (!confirm(`Remove ${card.name}? This only works if no movement references it.`)) return;
		cardsError = '';
		try {
			await removeCard(card.id);
		} catch (err) {
			cardsError = err.message;
		}
	}

	function startPay(card) {
		if (payingCardId === card.id) {
			payingCardId = null;
			return;
		}
		payingCardId = card.id;
		payAccountId = '';
		payAmountInput = card.next_due_total > 0 ? (card.next_due_total / 100).toFixed(2) : '';
		payError = '';
	}

	async function handlePaySubmit(card, event) {
		event.preventDefault();
		const cents = Math.round(parseFloat(payAmountInput) * 100);
		if (!cents || cents <= 0) {
			payError = 'Enter a positive amount';
			return;
		}
		if (!payAccountId) {
			payError = 'Choose the account the payment comes from';
			return;
		}
		payError = '';
		paySubmitting = true;
		try {
			// Convention (BACK-08): a card payment is an expense-shaped
			// movement — negative amount, category "transfer" — never a
			// literal /transfers call, since money is leaving an account to
			// settle a card statement, not moving between two accounts.
			// card_payment_for_card_id is what keeps it out of spend totals
			// (see MovementRow's card-payment chip).
			await createMovement({
				amount: -Math.abs(cents),
				currency: card.currency,
				description: `Payment — ${card.name}`,
				category: 'transfer',
				payment_method: 'bank_transfer',
				account_id: payAccountId,
				card_payment_for_card_id: card.id
			});
			payingCardId = null;
			notifyMovementCreated();
		} catch (err) {
			payError = err.message;
		} finally {
			paySubmitting = false;
		}
	}

	function payableAccounts(card) {
		return accounts.filter((a) => a.currency === card.currency);
	}

	function dueSoon(card) {
		if (card.next_due_total <= 0) return false;
		const days = (new Date(card.next_due_date) - new Date()) / (1000 * 60 * 60 * 24);
		return days >= 0 && days <= 7;
	}

	// Cards change whenever any movement is created anywhere in the app
	// (a credit-card purchase on `/`, a card payment from this sidebar) —
	// same movementCreated broadcast +page.svelte's own movement list
	// refreshes from. Skip the store's synchronous initial callback the
	// same way +page.svelte does, so mount doesn't double-fetch.
	let skippedInitialMovementEvent = false;
	const unsubscribeMovementCreated = movementCreated.subscribe(() => {
		if (!skippedInitialMovementEvent) {
			skippedInitialMovementEvent = true;
			return;
		}
		refreshCards();
	});
	onDestroy(unsubscribeMovementCreated);

	// A new account created on `/` (AccountsPanel) must show up in this
	// sidebar's own, independently-fetched account list too, or "Pay
	// card"'s picker silently stays stuck on stale data.
	let skippedInitialAccountEvent = false;
	const unsubscribeAccountsChanged = accountsChanged.subscribe(() => {
		if (!skippedInitialAccountEvent) {
			skippedInitialAccountEvent = true;
			return;
		}
		refreshAccounts();
	});
	onDestroy(unsubscribeAccountsChanged);

	onMount(() => {
		refreshCards();
		refreshAccounts();
		refreshCurrencies();
	});
</script>

<aside class="sidebar">
	<div class="brand">
		<span class="brand-mark">FT</span>
		<span class="brand-name">Financial Tracker</span>
	</div>

	<div class="section">
		<span class="section-label">Navigate</span>
		<ul class="nav-links">
			<li><a href="/">Movements</a></li>
			<li><a href="/import">Import CSV</a></li>
			<li><a href="/getting-started">Getting started</a></li>
		</ul>
	</div>

	<div class="section">
		<span class="section-label">Quick add</span>
		<ul class="quick-actions">
			{#each quickActions as action (action.label)}
				<li>
					<button type="button" onclick={() => handleQuickAction(action)}>
						<span class="icon">{action.icon}</span>
						<span class="label">{action.label}</span>
					</button>
				</li>
			{/each}
		</ul>
	</div>

	<div class="section">
		<div class="section-head-row">
			<span class="section-label">Cards</span>
			<button type="button" class="ghost small" onclick={() => (addingCard = !addingCard)}>
				{addingCard ? 'Close' : '+ Add card'}
			</button>
		</div>

		{#if addingCard}
			<CardForm
				{currencies}
				onSubmit={handleAddCard}
				onCancel={() => (addingCard = false)}
			/>
		{/if}

		{#if cardsError}
			<p class="message error">{cardsError}</p>
		{/if}

		{#if $cards.length === 0 && !addingCard}
			<p class="empty-hint">No cards registered yet.</p>
		{:else}
			<ul class="card-list">
				{#each $cards as card (card.id)}
					<li class="card-item">
						{#if editingCardId === card.id}
							<CardForm
								{card}
								{currencies}
								onSubmit={(payload) => handleEditCard(card.id, payload)}
								onCancel={() => (editingCardId = null)}
							/>
						{:else}
							<div class="card-item-head">
								<span class="card-name">{card.name}{card.last_four ? ` ••${card.last_four}` : ''}</span>
								<div class="card-item-actions">
									<button
										type="button"
										class="icon-btn"
										title="Edit card"
										onclick={() => (editingCardId = card.id)}>✎</button
									>
									<button
										type="button"
										class="icon-btn"
										title="Remove card"
										onclick={() => handleDeleteCard(card)}>✕</button
									>
								</div>
							</div>
							<div class="card-amounts">
								<span class="amount-line">
									Due {formatAmount(card.next_due_total, card.currency)}
									{#if dueSoon(card)}
										<span class="chip due-soon">
											due {new Date(card.next_due_date).toLocaleDateString(undefined, {
												day: 'numeric',
												month: 'short'
											})}
										</span>
									{/if}
								</span>
								<span class="amount-line">
									Open cycle {formatAmount(card.open_cycle_total, card.currency)}
								</span>
								{#if card.available_credit != null}
									<span class="amount-line">
										Available {formatAmount(card.available_credit, card.currency)}
									</span>
								{/if}
								{#if card.monthly_budget != null}
									{#if card.over_budget}
										<span class="chip over-budget">
											Over budget by {formatAmount(Math.abs(card.budget_remaining), card.currency)}
										</span>
									{:else}
										<span class="chip budget-ok">
											{formatAmount(card.budget_remaining, card.currency)} left this month
										</span>
									{/if}
								{/if}
							</div>

							<button type="button" class="ghost small" onclick={() => startPay(card)}>
								{payingCardId === card.id ? 'Close' : '+ Pay card'}
							</button>

							{#if payingCardId === card.id}
								<form class="pay-form" onsubmit={(e) => handlePaySubmit(card, e)}>
									<p class="hint">
										This records a payment from an account, not a new expense — it won't show up
										in spend totals.
									</p>
									<select bind:value={payAccountId} aria-label="Pay from account" required>
										<option value="" disabled>From account</option>
										{#each payableAccounts(card) as account (account.id)}
											<option value={account.id}>{account.name}</option>
										{/each}
									</select>
									<input
										type="number"
										step="0.01"
										min="0.01"
										placeholder="Amount"
										bind:value={payAmountInput}
										required
									/>
									{#if payableAccounts(card).length === 0}
										<p class="hint">No {card.currency.toUpperCase()} account to pay from yet.</p>
									{/if}
									{#if payError}
										<p class="message error">{payError}</p>
									{/if}
									<button class="submit" type="submit" disabled={paySubmitting}>
										{paySubmitting ? 'Paying…' : 'Pay card'}
									</button>
								</form>
							{/if}
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</aside>

<style>
	.sidebar {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		background: var(--color-surface);
		border-bottom: 1px solid var(--color-border);
		padding: var(--space-2);
	}

	.brand {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.brand-mark {
		display: grid;
		place-items: center;
		width: 2rem;
		height: 2rem;
		border-radius: var(--radius-control);
		background: linear-gradient(135deg, var(--color-primary), var(--color-secondary));
		color: #fff;
		font-weight: 700;
		font-size: 0.8rem;
		flex-shrink: 0;
	}

	.brand-name {
		font: var(--text-section-title);
		font-size: 0.95rem;
		color: var(--color-text-primary);
	}

	.section {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.section-label {
		font: var(--text-label);
		color: var(--color-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.section-head-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.4rem;
	}

	.quick-actions {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.nav-links {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.nav-links a {
		display: block;
		padding: 0.45rem 0.6rem;
		border-radius: var(--radius-control);
		color: var(--color-text-primary);
		text-decoration: none;
		font-size: 0.85rem;
		font-weight: 500;
		transition: background var(--transition-fast);
	}

	.nav-links a:hover {
		background: var(--color-bg);
	}

	.nav-links a:focus-visible {
		outline: none;
		box-shadow: var(--focus-ring);
	}

	.quick-actions button {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		background: var(--color-bg);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-control);
		padding: 0.45rem 0.7rem;
		color: var(--color-text-primary);
		transition:
			border-color var(--transition-fast),
			color var(--transition-fast);
	}

	.quick-actions button:hover {
		border-color: var(--color-secondary);
		color: var(--color-secondary);
	}

	.quick-actions button:focus-visible {
		outline: none;
		box-shadow: var(--focus-ring);
	}

	.icon {
		font-size: 1rem;
		line-height: 1;
	}

	.label {
		font-size: 0.82rem;
		font-weight: 500;
	}

	.ghost.small {
		padding: 0.3rem 0.6rem;
		font-size: 0.72rem;
	}

	.empty-hint {
		margin: 0;
		font-size: 0.8rem;
		color: var(--color-text-secondary);
	}

	.card-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.card-item {
		background: var(--color-bg);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-control);
		padding: 0.55rem 0.65rem;
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	.card-item-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.4rem;
	}

	.card-name {
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--color-text-primary);
	}

	.card-item-actions {
		display: flex;
		gap: 0.2rem;
	}

	.icon-btn {
		background: none;
		border: 1px solid var(--color-border);
		color: var(--color-text-secondary);
		border-radius: var(--radius-control);
		width: 1.6rem;
		height: 1.6rem;
		display: grid;
		place-items: center;
		font-size: 0.72rem;
	}

	.icon-btn:hover {
		color: var(--color-secondary);
		border-color: var(--color-secondary);
	}

	.card-amounts {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		font-size: 0.76rem;
		color: var(--color-text-secondary);
	}

	.amount-line {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		flex-wrap: wrap;
	}

	.chip {
		font: var(--text-label);
		font-size: 0.65rem;
		padding: 0.1rem 0.45rem;
		border-radius: var(--radius-pill);
		white-space: nowrap;
	}

	.chip.due-soon {
		background: #fef3c7;
		color: #92400e;
	}

	.chip.over-budget {
		background: var(--color-error-soft);
		color: var(--color-expense);
	}

	.chip.budget-ok {
		background: var(--color-success-soft);
		color: #166534;
	}

	.pay-form {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		margin-top: 0.3rem;
	}

	.hint {
		margin: 0;
		font-size: 0.72rem;
		color: var(--color-text-secondary);
	}

	@media (min-width: 860px) {
		.sidebar {
			flex-direction: column;
			width: 232px;
			flex-shrink: 0;
			height: 100vh;
			position: sticky;
			top: 0;
			overflow-y: auto;
			border-bottom: none;
			border-right: 1px solid var(--color-border);
		}

		.quick-actions {
			flex-direction: column;
		}

		.quick-actions button {
			width: 100%;
		}
	}
</style>
