<script>
	import '$lib/styles/tokens.css';
	import Sidebar from '$lib/components/Sidebar.svelte';

	let { children } = $props();
</script>

<div class="app-shell">
	<Sidebar />
	<div class="app-content">
		{@render children()}
	</div>
</div>

<style>
	:global(*),
	:global(*::before),
	:global(*::after) {
		box-sizing: border-box;
	}

	:global(body) {
		margin: 0;
		background: var(--color-bg);
	}

	.app-shell {
		display: flex;
		flex-direction: column;
		min-height: 100vh;
	}

	.app-content {
		flex: 1;
		min-width: 0;
	}

	@media (min-width: 860px) {
		.app-shell {
			flex-direction: row;
		}
	}

	:global(button) {
		font: inherit;
		cursor: pointer;
	}

	:global(.form-row) {
		display: flex;
		gap: 0.6rem;
	}

	:global(input),
	:global(select) {
		font: inherit;
		color: var(--color-text-primary);
		background: var(--color-bg);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-control);
		padding: 0.55rem 0.7rem;
		min-width: 0;
		transition:
			box-shadow var(--transition-fast),
			border-color var(--transition-fast);
	}

	:global(input:focus),
	:global(select:focus) {
		outline: none;
		border-color: var(--color-secondary);
		box-shadow: var(--focus-ring);
	}

	:global(.form-row input[type='number']),
	:global(.form-row input[type='text']),
	:global(.form-row select) {
		flex: 1;
	}

	:global(.submit) {
		background: var(--color-primary);
		color: #fff;
		border: none;
		border-radius: var(--radius-control);
		padding: 0.65rem;
		font-weight: 600;
		transition: background var(--transition-fast);
	}

	:global(.submit:hover:not(:disabled)) {
		background: var(--color-primary-hover);
	}

	:global(.submit:focus-visible) {
		outline: none;
		box-shadow: var(--focus-ring);
	}

	:global(.submit:disabled) {
		opacity: 0.6;
		cursor: default;
	}

	:global(.message) {
		border-radius: var(--radius-control);
		padding: 0.6rem 0.9rem;
		font-size: 0.9rem;
	}

	:global(.message.error) {
		background: var(--color-error-soft);
		color: var(--color-expense);
	}

	:global(.message.notice) {
		background: var(--color-success-soft);
		color: #166534;
	}

	:global(.card) {
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		box-shadow: var(--shadow-soft);
		padding: var(--space-2);
		margin-bottom: var(--space-3);
	}

	:global(.section-head) {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.6rem;
		flex-wrap: wrap;
	}

	:global(.section-head h2) {
		font: var(--text-section-title);
		font-size: 1.05rem;
		margin: 0;
		color: var(--color-text-primary);
		letter-spacing: -0.01em;
	}

	:global(.ghost) {
		background: none;
		border: 1px solid var(--color-border);
		color: var(--color-text-secondary);
		border-radius: var(--radius-control);
		padding: 0.4rem 0.75rem;
		font-size: 0.8rem;
		font-weight: 500;
		transition:
			color var(--transition-fast),
			border-color var(--transition-fast);
	}

	:global(.ghost:hover:not(:disabled)) {
		color: var(--color-secondary);
		border-color: var(--color-secondary);
	}

	:global(.ghost:focus-visible) {
		outline: none;
		box-shadow: var(--focus-ring);
	}

	:global(.fixed-currency) {
		display: flex;
		align-items: center;
		padding: 0 0.65rem;
		color: var(--color-text-secondary);
		border: 1px dashed var(--color-border);
		border-radius: var(--radius-control);
		font-size: 0.85rem;
	}

	:global(.empty.small) {
		padding: 0.9rem 0 0.2rem;
		font-size: 0.85rem;
		text-align: left;
	}
</style>
