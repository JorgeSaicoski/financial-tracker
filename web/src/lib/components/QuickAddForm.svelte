<script>
	import { onDestroy, onMount, tick } from 'svelte';
	import { categoryIcons, paymentMethodLabels, accountTypeIcons, labelFor } from '$lib/format.js';

	// mostRecent is the user's latest movement (or null — fresh account, no
	// history yet) used to prefill account/currency/payment method.
	// recentLoaded flips true once that fetch has settled (success or not),
	// so we can tell "still loading" apart from "loaded, genuinely empty"
	// and fall back to sane empty defaults instead of leaving fields stuck
	// on a loading state forever.
	let {
		categories,
		paymentMethods,
		currencies,
		accounts,
		mostRecent,
		recentCategories,
		recentDescriptions,
		recentLoaded,
		onSubmit
	} = $props();

	let amountEl = $state();
	let amountInput = $state('');
	let directionInput = $state('expense');
	let descriptionInput = $state('');
	let categoryInput = $state('other');
	let accountInput = $state('');
	let currencyInput = $state('usd');
	let paymentMethodInput = $state('other');
	let submitting = $state(false);
	let error = $state('');
	let justAdded = $state(false);
	let seeded = $state(false);
	let confirmTimeout;

	const selectedAccount = $derived(accounts.find((a) => a.id === accountInput));

	// Amount and category are the only fields that realistically need fresh
	// input every time (FRONT-09) — account/currency/payment method get
	// seeded once from the most recently used movement (or a sane empty
	// default on a fresh account) and are a one-tap override after that, not
	// re-seeded on every refetch so we don't clobber a mid-session override.
	$effect(() => {
		if (seeded || !recentLoaded) return;
		if (mostRecent) {
			accountInput = mostRecent.account_id ?? '';
			currencyInput = mostRecent.currency || currencies[0] || 'usd';
			paymentMethodInput = mostRecent.payment_method || 'other';
		} else if (currencies.length) {
			currencyInput = currencies[0];
		}
		seeded = true;
	});

	function selectCategory(cat) {
		categoryInput = cat;
	}

	function selectDescription(desc) {
		descriptionInput = desc;
	}

	async function handleSubmit(event) {
		event.preventDefault();

		const cents = Math.round(parseFloat(amountInput) * 100);
		if (!cents) {
			error = 'Enter a non-zero amount';
			return;
		}
		const signedAmount = directionInput === 'expense' ? -Math.abs(cents) : Math.abs(cents);

		submitting = true;
		error = '';
		try {
			await onSubmit({
				amount: signedAmount,
				currency: selectedAccount ? selectedAccount.currency : currencyInput,
				description: descriptionInput.trim(),
				category: categoryInput,
				payment_method: paymentMethodInput,
				installments: 1,
				account_id: accountInput || undefined
			});
			// Stay-open flow: reset only what genuinely needs fresh input
			// every time, keep the rest so a run of similar entries (a
			// grocery run, three coffees) is a tap + amount, not a
			// from-scratch form each time.
			amountInput = '';
			descriptionInput = '';
			justAdded = true;
			clearTimeout(confirmTimeout);
			confirmTimeout = setTimeout(() => (justAdded = false), 1800);
			await tick();
			amountEl?.focus();
		} catch (err) {
			error = err.message;
		} finally {
			submitting = false;
		}
	}

	onDestroy(() => clearTimeout(confirmTimeout));

	// Imperative focus instead of the autofocus attribute (a11y-lint flags
	// autofocus). Modal.svelte also grabs focus on mount (its close
	// button), via its own `await tick()`; a macrotask delay runs after
	// every pending microtask from both components' onMount, so the
	// amount input reliably wins that race and is ready to type into
	// immediately — the whole point of a *fast* expense entry.
	onMount(() => {
		const timer = setTimeout(() => amountEl?.focus(), 0);
		return () => clearTimeout(timer);
	});
</script>

<form onsubmit={handleSubmit} class="quick-add-form">
	<div class="amount-row">
		<div class="direction-toggle" role="group" aria-label="Direction">
			<button
				type="button"
				class:active={directionInput === 'expense'}
				aria-pressed={directionInput === 'expense'}
				onclick={() => (directionInput = 'expense')}
			>
				− Expense
			</button>
			<button
				type="button"
				class:active={directionInput === 'income'}
				aria-pressed={directionInput === 'income'}
				onclick={() => (directionInput = 'income')}
			>
				+ Income
			</button>
		</div>
		<input
			bind:this={amountEl}
			type="number"
			inputmode="decimal"
			step="0.01"
			min="0"
			placeholder="0.00"
			bind:value={amountInput}
			required
			class="amount-input"
			aria-label="Amount"
		/>
	</div>

	<div class="field">
		{#if recentCategories.length}
			<div class="chip-row" role="group" aria-label="Recent categories">
				{#each recentCategories as cat (cat)}
					<button
						type="button"
						class="chip-btn"
						class:selected={categoryInput === cat}
						aria-pressed={categoryInput === cat}
						onclick={() => selectCategory(cat)}
					>
						{categoryIcons[cat] ?? ''} {labelFor(cat)}
					</button>
				{/each}
			</div>
		{/if}
		<select bind:value={categoryInput} aria-label="Category">
			{#each categories as category (category)}
				<option value={category}>{categoryIcons[category] ?? ''} {labelFor(category)}</option>
			{/each}
		</select>
	</div>

	<div class="field">
		{#if recentDescriptions.length}
			<div class="chip-row" role="group" aria-label="Recent descriptions">
				{#each recentDescriptions as desc (desc)}
					<button
						type="button"
						class="chip-btn"
						class:selected={descriptionInput === desc}
						aria-pressed={descriptionInput === desc}
						onclick={() => selectDescription(desc)}
					>
						{desc}
					</button>
				{/each}
			</div>
		{/if}
		<input
			type="text"
			placeholder="Description (optional)"
			aria-label="Description"
			bind:value={descriptionInput}
		/>
	</div>

	<div class="overrides">
		<span class="overrides-label">Account · currency · payment method</span>
		<div class="form-row">
			<select bind:value={accountInput} aria-label="Account">
				<option value="">No account</option>
				{#each accounts as account (account.id)}
					<option value={account.id}>{accountTypeIcons[account.type] ?? ''} {account.name}</option>
				{/each}
			</select>
			{#if selectedAccount}
				<span class="fixed-currency" title="Currency follows the selected account">
					{selectedAccount.currency.toUpperCase()}
				</span>
			{:else}
				<select bind:value={currencyInput} aria-label="Currency">
					{#each currencies as c (c)}
						<option value={c}>{c.toUpperCase()}</option>
					{/each}
				</select>
			{/if}
		</div>
		<select bind:value={paymentMethodInput} aria-label="Payment method">
			{#each paymentMethods as method (method)}
				<option value={method}>{paymentMethodLabels[method] ?? method}</option>
			{/each}
		</select>
	</div>

	{#if error}
		<p class="message error">{error}</p>
	{/if}
	{#if justAdded}
		<p class="message notice" role="status">✓ Added — form's ready for the next one</p>
	{/if}

	<button class="submit" type="submit" disabled={submitting}>
		{#if submitting}Adding…{:else}Add{/if}
	</button>
</form>

<style>
	.quick-add-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding: var(--space-2);
	}

	.amount-row {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.direction-toggle {
		display: flex;
		gap: 0.4rem;
	}

	.direction-toggle button {
		flex: 1;
		min-height: 44px;
		background: var(--color-bg);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-control);
		color: var(--color-text-secondary);
		font-weight: 600;
		transition:
			background var(--transition-fast),
			color var(--transition-fast),
			border-color var(--transition-fast);
	}

	.direction-toggle button.active {
		background: var(--color-primary);
		border-color: var(--color-primary);
		color: #fff;
	}

	.direction-toggle button:focus-visible {
		outline: none;
		box-shadow: var(--focus-ring);
	}

	.amount-input {
		width: 100%;
		font-size: 1.9rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		text-align: center;
		padding: 0.75rem;
		min-height: 56px;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.chip-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}

	.chip-btn {
		min-height: 40px;
		background: var(--color-bg);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-pill);
		padding: 0.4rem 0.8rem;
		font-size: 0.85rem;
		font-weight: 500;
		color: var(--color-text-primary);
		transition:
			background var(--transition-fast),
			border-color var(--transition-fast);
	}

	.chip-btn.selected {
		background: var(--color-secondary);
		border-color: var(--color-secondary);
		color: #fff;
	}

	.chip-btn:focus-visible {
		outline: none;
		box-shadow: var(--focus-ring);
	}

	.overrides {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding-top: 0.3rem;
		border-top: 1px dashed var(--color-border);
	}

	.overrides-label {
		font: var(--text-label);
		color: var(--color-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.submit {
		min-height: 48px;
		font-size: 1.05rem;
	}
</style>
