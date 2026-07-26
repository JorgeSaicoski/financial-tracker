<script>
	import { authState, login } from '$lib/auth.svelte.js';

	let redirecting = $state(false);

	async function handleLogin() {
		redirecting = true;
		try {
			await login();
		} catch (err) {
			authState.error = err.message;
			redirecting = false;
		}
	}
</script>

<div class="login-screen">
	<div class="login-card card">
		<h1>Financial Tracker</h1>
		<p>Sign in with your Authentik account to continue.</p>

		{#if authState.error}
			<p class="message error">{authState.error}</p>
		{/if}

		<button class="submit" onclick={handleLogin} disabled={redirecting}>
			{#if redirecting}Redirecting…{:else}Log in{/if}
		</button>
	</div>
</div>

<style>
	.login-screen {
		min-height: 100vh;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-3);
		background: var(--color-bg);
	}

	.login-card {
		max-width: 360px;
		width: 100%;
		text-align: center;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.login-card h1 {
		font: var(--text-page-title);
		margin: 0;
		color: var(--color-primary);
		letter-spacing: -0.02em;
	}

	.login-card p {
		font: var(--text-body);
		color: var(--color-text-secondary);
		margin: 0;
	}

	.login-card .submit {
		width: 100%;
	}
</style>
