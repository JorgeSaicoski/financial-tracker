<script>
	// Quick-add and Cards are UI-only for now — wiring is FRONT-07
	// (quick-add needs shared state with the page; Cards needs BACK-08's
	// card entity, which doesn't exist yet). Both render disabled with a
	// "soon" badge until that ticket lands.
	const quickActions = [
		{ label: 'Income', icon: '💰' },
		{ label: 'Market', icon: '🍽️' },
		{ label: 'Shop', icon: '🛍️' },
		{ label: 'Bill', icon: '💡' }
	];
</script>

<aside class="sidebar">
	<div class="brand">
		<span class="brand-mark">FT</span>
		<span class="brand-name">Financial Tracker</span>
	</div>

	<div class="section">
		<span class="section-label">Quick add</span>
		<ul class="quick-actions">
			{#each quickActions as action (action.label)}
				<li>
					<button type="button" disabled title="Coming soon — FRONT-07">
						<span class="icon">{action.icon}</span>
						<span class="label">{action.label}</span>
						<span class="soon">soon</span>
					</button>
				</li>
			{/each}
		</ul>
	</div>

	<div class="section">
		<span class="section-label">Cards</span>
		<p class="empty-hint">No cards registered yet.</p>
		<button type="button" class="add-card" disabled title="Coming soon — BACK-08 + FRONT-07">
			<span class="icon">💳</span>
			<span class="label">Add card</span>
			<span class="soon">soon</span>
		</button>
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

	.quick-actions {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	button[disabled] {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		background: var(--color-bg);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-control);
		padding: 0.45rem 0.7rem;
		color: var(--color-text-secondary);
		opacity: 0.7;
		cursor: not-allowed;
	}

	.add-card {
		width: 100%;
		justify-content: flex-start;
	}

	.icon {
		font-size: 1rem;
		line-height: 1;
	}

	.label {
		font-size: 0.82rem;
		font-weight: 500;
	}

	.soon {
		font: var(--text-label);
		font-size: 0.62rem;
		background: #fef3c7;
		color: #92400e;
		padding: 0.1rem 0.4rem;
		border-radius: var(--radius-pill);
		margin-left: auto;
	}

	.empty-hint {
		margin: 0;
		font-size: 0.8rem;
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
			border-bottom: none;
			border-right: 1px solid var(--color-border);
		}

		.quick-actions {
			flex-direction: column;
		}

		button[disabled] {
			width: 100%;
		}
	}
</style>
