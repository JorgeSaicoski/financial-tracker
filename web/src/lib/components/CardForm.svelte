<script>
	// Shared by Sidebar's "+ Add card" and per-card "Edit" toggle — card is
	// undefined in add mode (all fields start blank/default) or a
	// CardResponse in edit mode (fields prefilled, currency locked since
	// PATCH /cards/{id} has no currency field).
	let { card, currencies, onSubmit, onCancel } = $props();

	const dayOptions = [...Array.from({ length: 28 }, (_, i) => String(i + 1)), 'last'];

	let nameInput = $state(card?.name ?? '');
	let lastFourInput = $state(card?.last_four ?? '');
	let closingDayInput = $state(card?.closing_day ?? '1');
	let dueDayInput = $state(card?.due_day ?? '10');
	let creditLimitInput = $state(card?.credit_limit != null ? String(card.credit_limit / 100) : '');
	let monthlyBudgetInput = $state(card?.monthly_budget != null ? String(card.monthly_budget / 100) : '');
	let currencyInput = $state(card?.currency ?? currencies[0] ?? 'usd');
	let submitting = $state(false);
	let error = $state('');

	function toCents(input) {
		if (input === '' || input == null) return undefined;
		const n = Math.round(parseFloat(input) * 100);
		return Number.isFinite(n) ? n : undefined;
	}

	async function handleSubmit(event) {
		event.preventDefault();
		error = '';
		submitting = true;
		try {
			const payload = {
				name: nameInput.trim(),
				last_four: lastFourInput.trim(),
				closing_day: closingDayInput,
				due_day: dueDayInput,
				credit_limit: toCents(creditLimitInput),
				monthly_budget: toCents(monthlyBudgetInput)
			};
			if (!card) payload.currency = currencyInput;
			await onSubmit(payload);
		} catch (err) {
			error = err.message;
		} finally {
			submitting = false;
		}
	}
</script>

<form class="card-form" onsubmit={handleSubmit}>
	<div class="form-row">
		<input type="text" placeholder="Card name" bind:value={nameInput} required />
		<input type="text" placeholder="Last 4 (optional)" maxlength="4" bind:value={lastFourInput} />
	</div>
	<div class="form-row">
		<label class="day-field">
			<span>Closing day</span>
			<select bind:value={closingDayInput} aria-label="Closing day">
				{#each dayOptions as day (day)}
					<option value={day}>{day === 'last' ? 'Last day' : day}</option>
				{/each}
			</select>
		</label>
		<label class="day-field">
			<span>Due day</span>
			<select bind:value={dueDayInput} aria-label="Due day">
				{#each dayOptions as day (day)}
					<option value={day}>{day === 'last' ? 'Last day' : day}</option>
				{/each}
			</select>
		</label>
	</div>
	<div class="form-row">
		<input type="number" step="0.01" min="0" placeholder="Credit limit (optional)" bind:value={creditLimitInput} />
		<input
			type="number"
			step="0.01"
			min="0"
			placeholder="Monthly budget (optional)"
			bind:value={monthlyBudgetInput}
		/>
	</div>
	{#if !card}
		<div class="form-row">
			<select bind:value={currencyInput} aria-label="Currency">
				{#each currencies as c (c)}
					<option value={c}>{c.toUpperCase()}</option>
				{/each}
			</select>
		</div>
	{/if}

	{#if error}
		<p class="message error">{error}</p>
	{/if}

	<div class="form-actions">
		<button class="submit" type="submit" disabled={submitting}>
			{submitting ? 'Saving…' : card ? 'Save changes' : 'Add card'}
		</button>
		<button type="button" class="ghost" onclick={onCancel}>Cancel</button>
	</div>
</form>

<style>
	.card-form {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin: 0.4rem 0;
	}

	.day-field {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		font-size: 0.72rem;
		color: var(--color-text-secondary);
	}

	.form-actions {
		display: flex;
		gap: 0.4rem;
	}

	.form-actions .submit {
		flex: 1;
	}
</style>
