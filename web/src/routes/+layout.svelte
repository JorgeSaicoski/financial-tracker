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
</style>
