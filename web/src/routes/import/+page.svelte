<script>
	import { onMount } from 'svelte';
	import { getImportSpec, importMovements } from '$lib/api.js';
	import { formatAmount } from '$lib/format.js';

	let spec = $state(null);
	let loadingSpec = $state(true);
	let specError = $state('');

	let csvText = $state('');
	let fileName = $state('');
	let allowPartial = $state(false);
	let skipDuplicates = $state(false);

	let previewResult = $state(null);
	let finalResult = $state(null);
	let previewing = $state(false);
	let confirming = $state(false);
	let error = $state('');
	let notice = $state('');
	let showPrompt = $state(false);

	const columnNames = $derived(spec?.columns.map((c) => c.name) ?? []);

	// Client-side CSV parsing purely for the preview table (row-by-row
	// display, per-currency sums) — the backend is still the only source
	// of truth for validation; this never decides what's valid.
	function parseCSVBody(text) {
		const rows = [];
		let field = '';
		let row = [];
		let inQuotes = false;
		for (let i = 0; i < text.length; i++) {
			const c = text[i];
			if (inQuotes) {
				if (c === '"') {
					if (text[i + 1] === '"') {
						field += '"';
						i++;
					} else {
						inQuotes = false;
					}
				} else {
					field += c;
				}
				continue;
			}
			if (c === '"') {
				inQuotes = true;
			} else if (c === ',') {
				row.push(field);
				field = '';
			} else if (c === '\r') {
				// skip — \n (below) closes the row
			} else if (c === '\n') {
				row.push(field);
				rows.push(row);
				row = [];
				field = '';
			} else {
				field += c;
			}
		}
		if (field.length > 0 || row.length > 0) {
			row.push(field);
			rows.push(row);
		}
		return rows.filter((r) => !(r.length === 1 && r[0].trim() === ''));
	}

	const parsedRows = $derived.by(() => {
		if (!csvText.trim() || columnNames.length === 0) return [];
		const allRows = parseCSVBody(csvText.trim());
		if (allRows.length === 0) return [];
		const first = allRows[0].map((v) => v.trim().toLowerCase());
		const isHeader = columnNames.every((name, idx) => first[idx] === name);
		return isHeader ? allRows.slice(1) : allRows;
	});

	const currencyTotals = $derived.by(() => {
		const totals = {};
		for (const row of parsedRows) {
			const currency = (row[2] ?? '').trim().toLowerCase();
			const amount = parseInt(row[1], 10);
			if (!currency || Number.isNaN(amount)) continue;
			totals[currency] ??= { positive: 0, negative: 0, count: 0 };
			if (amount >= 0) totals[currency].positive += amount;
			else totals[currency].negative += amount;
			totals[currency].count += 1;
		}
		return totals;
	});

	const errorsByRow = $derived.by(() => {
		const map = {};
		for (const e of previewResult?.errors ?? []) {
			(map[e.row] ??= []).push(e);
		}
		return map;
	});

	const duplicatesByRow = $derived.by(() => {
		const map = {};
		for (const d of previewResult?.duplicates ?? []) {
			map[d.row] = d;
		}
		return map;
	});

	const hasBlockingErrors = $derived(
		(previewResult?.errors?.length ?? 0) > 0 && !allowPartial
	);
	const canConfirm = $derived(Boolean(previewResult) && !hasBlockingErrors && parsedRows.length > 0);

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

	async function handleFileChange(event) {
		const file = event.target.files?.[0];
		if (!file) return;
		fileName = file.name;
		csvText = await file.text();
		previewResult = null;
		finalResult = null;
	}

	function handleTextareaInput() {
		fileName = '';
		previewResult = null;
		finalResult = null;
	}

	function downloadTemplate() {
		if (!spec?.template) return;
		const blob = new Blob([spec.template], { type: 'text/csv' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = 'financial-tracker-import-template.csv';
		a.click();
		URL.revokeObjectURL(url);
	}

	function buildPrompt() {
		if (!spec) return '';
		const columnLines = spec.columns
			.map((c) => {
				const req = c.required ? 'required' : 'optional';
				const allowed = c.allowed_values?.length
					? ` — allowed values: ${c.allowed_values.join(', ')}`
					: '';
				return `- ${c.name} (${c.type}, ${req})${allowed}`;
			})
			.join('\n');
		return `Convert my bank statement below into a CSV with exactly these columns, in this exact order, with this exact header row as the first line:\n${columnNames.join(',')}\n\nColumn rules:\n${columnLines}\n\nOutput only the CSV — no explanation, no markdown code fences, nothing else.\n\nHere is my bank statement:\n[paste your statement here]`;
	}

	async function copyPrompt() {
		error = '';
		notice = '';
		try {
			await navigator.clipboard.writeText(buildPrompt());
			notice = 'Prompt copied — paste it into your AI of choice along with your bank statement.';
		} catch {
			showPrompt = true;
			error = "Couldn't copy automatically — select the text below and copy it manually.";
		}
	}

	async function handlePreview() {
		error = '';
		notice = '';
		finalResult = null;
		previewing = true;
		try {
			previewResult = await importMovements(csvText, {
				dryRun: true,
				allowPartial,
				skipDuplicates
			});
		} catch (err) {
			error = err.message;
			previewResult = null;
		} finally {
			previewing = false;
		}
	}

	async function handleConfirm() {
		error = '';
		notice = '';
		confirming = true;
		try {
			finalResult = await importMovements(csvText, {
				dryRun: false,
				allowPartial,
				skipDuplicates
			});
			previewResult = null;
		} catch (err) {
			error = err.message;
		} finally {
			confirming = false;
		}
	}

	function startOver() {
		csvText = '';
		fileName = '';
		previewResult = null;
		finalResult = null;
		error = '';
		notice = '';
	}

	onMount(() => {
		loadSpec();
	});
</script>

<main>
	<h1>Import history from CSV</h1>
	<p class="intro">
		Bring your history from day one: copy the spec below, hand it to any AI along with your bank
		statement, and upload the CSV it gives back.
	</p>

	{#if loadingSpec}
		<p class="empty">Loading import spec…</p>
	{:else if specError}
		<p class="message error">{specError}</p>
	{:else if spec}
		<section class="card">
			<div class="section-head">
				<h2>1. The format</h2>
				<div class="actions">
					<button class="ghost" onclick={downloadTemplate}>Download template CSV</button>
					<button class="ghost" onclick={copyPrompt}>Copy AI prompt</button>
				</div>
			</div>

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

			{#if showPrompt}
				<div class="prompt-fallback">
					<p class="hint">Copy this prompt manually:</p>
					<textarea readonly rows="8" value={buildPrompt()}></textarea>
				</div>
			{/if}
		</section>

		<section class="card">
			<div class="section-head">
				<h2>2. Upload your CSV</h2>
			</div>

			<div class="upload-row">
				<label class="file-picker">
					<input type="file" accept=".csv,text/csv" onchange={handleFileChange} />
					{fileName || 'Choose a file…'}
				</label>
				<span class="or">or paste it below</span>
			</div>

			<textarea
				class="csv-textarea"
				rows="8"
				placeholder={spec.template}
				bind:value={csvText}
				oninput={handleTextareaInput}
			></textarea>

			<div class="toggles">
				<label>
					<input type="checkbox" bind:checked={allowPartial} />
					Allow partial import (import valid rows, skip the rest)
				</label>
				<label>
					<input type="checkbox" bind:checked={skipDuplicates} />
					Skip duplicates on import
				</label>
			</div>

			<button class="submit" onclick={handlePreview} disabled={!csvText.trim() || previewing}>
				{previewing ? 'Checking…' : 'Preview import'}
			</button>
		</section>

		{#if previewResult}
			<section class="card">
				<div class="section-head">
					<h2>3. Preview</h2>
				</div>

				<div class="summary">
					<span><strong>{parsedRows.length}</strong> rows parsed</span>
					<span><strong>{previewResult.errors.length}</strong> errors</span>
					<span><strong>{previewResult.duplicates.length}</strong> duplicates</span>
					<span>would import <strong>{previewResult.imported}</strong>, skip <strong>{previewResult.skipped}</strong></span>
				</div>

				{#if Object.keys(currencyTotals).length > 0}
					<table class="totals-table">
						<thead>
							<tr>
								<th>Currency</th>
								<th>Rows</th>
								<th>Income</th>
								<th>Expense</th>
							</tr>
						</thead>
						<tbody>
							{#each Object.entries(currencyTotals) as [currency, t] (currency)}
								<tr>
									<td class="mono">{currency.toUpperCase()}</td>
									<td>{t.count}</td>
									<td class="pos">{formatAmount(t.positive, currency)}</td>
									<td class="neg">{formatAmount(Math.abs(t.negative), currency)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}

				{#if hasBlockingErrors}
					<p class="message error">
						This file has row errors and strict mode is on — nothing will import. Fix the file or
						enable "allow partial import" above, then preview again.
					</p>
				{/if}

				<div class="rows-table-wrap">
					<table class="rows-table">
						<thead>
							<tr>
								<th>Row</th>
								{#each columnNames as name (name)}
									<th>{name}</th>
								{/each}
								<th>Status</th>
							</tr>
						</thead>
						<tbody>
							{#each parsedRows as row, idx (idx)}
								{@const rowNum = idx + 1}
								{@const rowErrors = errorsByRow[rowNum]}
								{@const duplicate = duplicatesByRow[rowNum]}
								<tr class:error-row={rowErrors} class:duplicate-row={duplicate && !rowErrors}>
									<td>{rowNum}</td>
									{#each row as value, i (i)}
										<td>{value}</td>
									{/each}
									<td>
										{#if rowErrors}
											{#each rowErrors as e (e.field + e.message)}
												<span class="row-flag error-flag">{e.field}: {e.message}</span>
											{/each}
										{:else if duplicate}
											<span class="row-flag duplicate-flag">
												{duplicate.existing_movement_id
													? 'looks identical to an existing movement'
													: `duplicate of row ${duplicate.duplicate_of_row}`}
											</span>
										{:else}
											<span class="row-flag ok-flag">ok</span>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<button class="submit" onclick={handleConfirm} disabled={!canConfirm || confirming}>
					{confirming ? 'Importing…' : 'Confirm import'}
				</button>
			</section>
		{/if}

		{#if finalResult}
			<section class="card">
				<div class="section-head">
					<h2>Done</h2>
				</div>
				<p class="message notice">
					Imported {finalResult.imported}, skipped {finalResult.skipped}.
				</p>
				<div class="actions">
					<a class="ghost" href="/">Back to movements</a>
					<button class="ghost" onclick={startOver}>Import another file</button>
				</div>
			</section>
		{/if}

		{#if error}
			<p class="message error">{error}</p>
		{/if}
		{#if notice}
			<p class="message notice">{notice}</p>
		{/if}
	{/if}
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

	.actions {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.spec-table-wrap,
	.rows-table-wrap {
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

	.prompt-fallback {
		margin-top: var(--space-2);
	}

	.prompt-fallback textarea {
		width: 100%;
		font-family: ui-monospace, monospace;
		font-size: 0.8rem;
	}

	.hint {
		margin: 0 0 0.3rem;
		font-size: 0.78rem;
		color: var(--color-text-secondary);
	}

	.upload-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
		margin-top: var(--space-2);
	}

	.file-picker {
		position: relative;
		display: inline-flex;
		align-items: center;
		background: var(--color-bg);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-control);
		padding: 0.55rem 0.9rem;
		font-size: 0.85rem;
		cursor: pointer;
	}

	.file-picker input {
		position: absolute;
		inset: 0;
		opacity: 0;
		cursor: pointer;
	}

	.or {
		font-size: 0.78rem;
		color: var(--color-text-secondary);
	}

	.csv-textarea {
		width: 100%;
		margin-top: var(--space-2);
		font-family: ui-monospace, monospace;
		font-size: 0.8rem;
	}

	.toggles {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		margin: var(--space-2) 0;
		font-size: 0.85rem;
	}

	.toggles label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.summary {
		display: flex;
		gap: 1rem;
		flex-wrap: wrap;
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		margin-bottom: var(--space-2);
	}

	.totals-table {
		margin-bottom: var(--space-2);
	}

	.pos {
		color: var(--color-income);
		font-variant-numeric: tabular-nums;
	}

	.neg {
		color: var(--color-expense);
		font-variant-numeric: tabular-nums;
	}

	.error-row {
		background: var(--color-error-soft);
	}

	.duplicate-row {
		background: #fef3c7;
	}

	.row-flag {
		display: inline-block;
		font-size: 0.72rem;
		padding: 0.1rem 0.4rem;
		border-radius: var(--radius-pill);
		margin: 0.1rem 0.2rem 0.1rem 0;
	}

	.error-flag {
		background: var(--color-error-soft);
		color: var(--color-expense);
	}

	.duplicate-flag {
		background: #fef3c7;
		color: #92400e;
	}

	.ok-flag {
		background: var(--color-success-soft);
		color: #166534;
	}
</style>
