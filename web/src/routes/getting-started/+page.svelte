<script>
	import { onMount } from 'svelte';
	import { getImportSpec } from '$lib/api.js';

	// Same live-spec pattern as /import (FRONT-01): this page never
	// hardcodes a second copy of BACK-03's column model, so a spec change
	// there updates what's shown here with no code change on this page.
	let spec = $state(null);
	let loadingSpec = $state(true);
	let specError = $state('');

	async function loadSpec() {
		specError = '';
		loadingSpec = true;
		try {
			spec = await getImportSpec();
		} catch (err) {
			specError = err.message;
		} finally {
			loadingSpec = false;
		}
	}

	onMount(() => {
		loadSpec();
	});
</script>

<main>
	<h1>Getting started</h1>
	<p class="intro">A couple of things worth knowing before you dive in.</p>

	<section class="card">
		<div class="section-head">
			<h2>Bring your history from day one</h2>
		</div>
		<p>
			You don't have to start from a blank slate. Export a statement from your bank (or just
			screenshot it), hand it to any AI you already use — ChatGPT, Claude, Gemini, whatever —
			along with the column spec below, and it'll convert it into a CSV in our format. Upload
			that on the <a href="/import">import page</a> and your history is there from day one.
		</p>
		<p class="optional-callout">
			<strong>This step is entirely optional.</strong> Starting from zero and logging movements
			going forward works fine too — nothing about the app assumes you imported anything.
		</p>

		{#if loadingSpec}
			<p class="empty">Loading the import spec…</p>
		{:else if specError}
			<p class="message error">{specError}</p>
		{:else if spec}
			<div class="spec-table-wrap">
				<table class="spec-table">
					<thead>
						<tr>
							<th>Column</th>
							<th>Type</th>
							<th>Required</th>
							<th>Allowed values</th>
						</tr>
					</thead>
					<tbody>
						{#each spec.columns as column (column.name)}
							<tr>
								<td class="mono">{column.name}</td>
								<td>{column.type}</td>
								<td>{column.required ? 'Yes' : 'No'}</td>
								<td>
									{#if column.allowed_values?.length}
										{column.allowed_values.join(', ')}
									{:else}
										<span class="muted">any</span>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}

		<a class="submit start-import" href="/import">Start the import →</a>
	</section>

	<!--
		BACK-15 (local-only encrypted archive) has a PR open but unmerged;
		BACK-16 (cloud tier) and BACK-19 (paid subscription) haven't
		started at all. Per this ticket's own instruction, the
		privacy/pricing section stays a "coming soon" placeholder — never
		describing unshipped storage-tier behavior in present tense —
		until all three actually land.
	-->
	<section class="card coming-soon">
		<div class="section-head">
			<h2>Choose how much we get to see</h2>
			<span class="soon-badge">Coming soon</span>
		</div>
		<p class="muted">
			Local-only, cloud-backed, and audit-trail storage tiers are on the way — you'll pick
			exactly how much of your data we ever see, at a price shown in your own currency before
			you commit to anything. Nothing here is live yet.
		</p>
	</section>
</main>

<style>
	main {
		max-width: 860px;
		margin: 0 auto;
		padding: var(--space-4) var(--space-2) var(--space-6);
		font: var(--text-body);
		color: var(--color-text-primary);
	}

	h1 {
		font: var(--text-page-title);
		margin: 0 0 var(--space-1);
		color: var(--color-primary);
		letter-spacing: -0.02em;
	}

	.intro {
		color: var(--color-text-secondary);
		margin: 0 0 var(--space-3);
		max-width: 60ch;
	}

	.optional-callout {
		background: var(--color-success-soft);
		color: #166534;
		border-radius: var(--radius-control);
		padding: 0.6rem 0.9rem;
		font-size: 0.9rem;
	}

	.spec-table-wrap {
		overflow-x: auto;
		margin-top: var(--space-2);
	}

	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.85rem;
	}

	th,
	td {
		text-align: left;
		padding: 0.5rem 0.6rem;
		border-bottom: 1px solid var(--color-border);
		white-space: nowrap;
	}

	th {
		color: var(--color-text-secondary);
		font-weight: 600;
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.mono {
		font-family: ui-monospace, monospace;
	}

	.muted {
		color: var(--color-text-secondary);
	}

	.start-import {
		display: inline-block;
		text-decoration: none;
		margin-top: var(--space-3);
		width: auto;
		padding: 0.65rem 1.1rem;
	}

	.coming-soon {
		opacity: 0.9;
	}

	.soon-badge {
		font: var(--text-label);
		font-size: 0.7rem;
		background: #fef3c7;
		color: #92400e;
		padding: 0.15rem 0.5rem;
		border-radius: var(--radius-pill);
	}
</style>
