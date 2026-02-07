<script>
	import Sidebar from "$lib/components/Sidebar.svelte";
	import TaskList from "$lib/components/TaskList.svelte";
	import TaskChat from "$lib/components/TaskChat.svelte";
	import TaskModal from "$lib/components/TaskModal.svelte";
	import { onMount } from "svelte";

	let selectedProject = $state("");
	let selectedUser = $state("");
	/** @type {string[]} */
	let logs = $state([]);
	/** @type {any[]} */
	let tasks = $state([]);
	/** @type {any[]} */
	let stages = $state([]);
	let date = $state("");
	let report_html = $state("");

	let isChatOpen = $state(false);
	let editingTaskIndex = $state(-1);
	let editingTask = $state(null);

	/** @type {any[]} */
	let projects = $state([]);
	let settings = $state({ usernames: [], project_paths: [] });

	function getTodayString() {
		const d = new Date();
		const yyyy = d.getFullYear();
		const mm = String(d.getMonth() + 1).padStart(2, "0");
		const dd = String(d.getDate()).padStart(2, "0");
		return `${yyyy}-${mm}-${dd}`;
	}

	async function loadSettings() {
		try {
			const res = await fetch("/api/settings");
			if (res.ok) {
				const data = await res.json();
				settings = data.settings;
				projects = data.projects;
				if (!selectedProject && data.current_project) {
					selectedProject = data.current_project;
				}
			}
		} catch (e) {
			console.error("Failed to load settings", e);
		}
	}

	async function loadState() {
		try {
			const res = await fetch("/api/state");
			if (res.ok) {
				const state = await res.json();
				logs = state.logs || [];
				stages = state.stages || [];

				// Only update tasks/report if the date still matches
				// to avoid race conditions when switching dates
				if (state.date === date) {
					console.log(
						`[loadState] Date matches (${state.date}), updating tasks:`,
						state.tasks?.length || 0,
					);
					tasks = state.tasks || [];
					report_html = state.report_html || "";
				} else {
					console.log(
						`[loadState] Date mismatch: state.date=${state.date}, ui.date=${date}, skipping task update`,
					);
				}

				if (!date && state.date) date = state.date;
			}
		} catch (e) {
			console.error("Failed to load state", e);
		}
	}

	onMount(() => {
		if (!date) {
			date = getTodayString();
		}
		loadSettings();
		loadState();
		const interval = setInterval(loadState, 2000);
		return () => clearInterval(interval);
	});

	$effect(() => {
		if (date && selectedProject) {
			console.log(
				`[loadHistory] Fetching tasks for date=${date}, repo=${selectedProject}`,
			);
			loadHistory();
		}
	});

	async function loadHistory() {
		try {
			const res = await fetch(
				`/api/load-history?date=${date}&repo=${encodeURIComponent(selectedProject)}`,
			);
			if (res.ok) {
				const data = await res.json();
				console.log(
					`[loadHistory] Received ${data?.length || 0} tasks:`,
					data,
				);
				tasks = data || [];
			} else {
				console.error(`[loadHistory] Failed with status ${res.status}`);
			}
		} catch (e) {
			console.error("Failed to load history", e);
		}
	}

	async function handleClearTasks() {
		if (!date || !selectedProject) return;
		if (!confirm("Are you sure you want to clear tasks for this day?"))
			return;
		try {
			const res = await fetch(
				`/api/clear-tasks?date=${date}&repo=${encodeURIComponent(selectedProject)}`,
				{ method: "POST" }, // Though handler in go doesn't strictly check method yet, it's good practice
			);
			if (res.ok) {
				tasks = [];
				report_html = "";
			}
		} catch (e) {
			console.error("Failed to clear tasks", e);
		}
	}

	async function handleAddProject() {
		const path = prompt("Enter absolute path to git repository:");
		if (!path) return;
		try {
			const res = await fetch("/api/settings", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({
					project_paths: [...settings.project_paths, path],
					usernames: settings.usernames,
				}),
			});
			if (res.ok) {
				await loadSettings();
			}
		} catch (e) {
			console.error("Failed to add project", e);
		}
	}

	async function handleScanUsers() {
		if (!selectedProject) return;
		try {
			const res = await fetch("/api/scan-users", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ path: selectedProject }),
			});
			if (res.ok) {
				const data = await res.json();
				// Merge and save
				const combined = Array.from(
					new Set([...(settings.usernames || []), ...data.usernames]),
				);
				await fetch("/api/settings", {
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({
						project_paths: settings.project_paths,
						usernames: combined,
					}),
				});
				await loadSettings();
			}
		} catch (e) {
			console.error("Failed to scan users", e);
		}
	}

	async function handleRun() {
		if (!date) {
			alert("Please select a date");
			return;
		}
		try {
			const res = await fetch("/api/run", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({
					date,
					repo_path: selectedProject,
					author: selectedUser,
				}),
			});
			if (res.ok) {
				// Reset stages locally for immediate feedback
				stages = stages.map((s) => ({ ...s, status: "pending" }));
				loadState();
			} else {
				const err = await res.text();
				alert("Run failed: " + err);
			}
		} catch (e) {
			console.error("Failed to run", e);
		}
	}

	/** @param {any} commit */
	function handleCommitClick(commit) {
		const d = new Date(commit.date * 1000);
		const yyyy = d.getFullYear();
		const mm = String(d.getMonth() + 1).padStart(2, "0");
		const dd = String(d.getDate()).padStart(2, "0");
		date = `${yyyy}-${mm}-${dd}`;
		// No automatic run as requested
	}

	/**
	 * @param {number} index
	 * @param {any} task
	 */
	async function handleUpdateTask(index, task) {
		try {
			const res = await fetch("/api/update-task", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ index, task }),
			});
			if (res.ok) {
				const updated = await res.json();
				tasks = updated;
				// Refresh report
				const reportRes = await fetch("/api/state");
				const state = await reportRes.json();
				report_html = state.report_html;
			}
		} catch (e) {
			console.error("Failed to update task", e);
		}
	}

	async function handleSend() {
		try {
			const res = await fetch("/api/send", { method: "POST" });
			if (res.ok) {
				alert("Report sent to Slack!");
			} else {
				const errorTxt = await res.text();
				alert("Failed to send: " + errorTxt);
			}
		} catch (e) {
			console.error("Failed to send", e);
		}
	}
	/** @param {number} index @param {string} action */
	async function handleTaskAction(index, action) {
		if (action === "manual_edit") {
			editingTaskIndex = index;
			editingTask = JSON.parse(JSON.stringify(tasks[index]));
			return;
		}

		try {
			// Update status to show something is happening
			stages = stages.map((s, i) =>
				i === 3 ? { ...s, status: "running" } : s,
			);

			const res = await fetch("/api/action", {
				method: "POST",
				body: JSON.stringify({ action, selected: [index] }),
			});
			if (res.ok) {
				const updatedTasks = await res.json();
				tasks = updatedTasks;
				stages = stages.map((s, i) =>
					i === 3 ? { ...s, status: "done", note: "Refined" } : s,
				);
				// Also refresh report
				const reportRes = await fetch("/api/state");
				const state = await reportRes.json();
				report_html = state.report_html;
			}
		} catch (e) {
			console.error("Action failed", e);
		}
	}
</script>

<div class="flex h-screen bg-[#080a0d] text-gray-100 overflow-hidden font-sans">
	<Sidebar
		appState={{ selectedProject, selectedUser, logs }}
		{projects}
		{settings}
		onSelectProject={(/** @type {string} */ path) =>
			(selectedProject = path)}
		onSelectUser={(/** @type {string} */ user) => (selectedUser = user)}
		onAddProject={handleAddProject}
		onScanUsers={handleScanUsers}
		onCommitClick={handleCommitClick}
	/>

	{#if isChatOpen}
		<TaskChat
			isOpen={isChatOpen}
			onClose={() => (isChatOpen = false)}
			onTasksUpdate={(updated) => {
				tasks = updated;
				// trigger refresh
				loadState();
			}}
		/>
	{/if}

	{#if editingTaskIndex >= 0}
		<TaskModal
			task={editingTask}
			index={editingTaskIndex}
			onClose={() => {
				editingTaskIndex = -1;
				editingTask = null;
			}}
			onSave={(idx, updated) => handleUpdateTask(idx, updated)}
		/>
	{/if}

	<main class="flex-1 flex flex-col min-w-0 overflow-hidden bg-[#0d1117]/50">
		<header
			class="h-16 border-b border-white/10 flex items-center justify-between px-8 shrink-0 bg-[#0d1117]"
		>
			<div class="flex items-center gap-4">
				<img src="/logo.png" alt="ssbot logo" class="h-6" />
				<button
					onclick={() => (isChatOpen = !isChatOpen)}
					class="px-3 py-1.5 text-xs font-medium rounded-lg bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 hover:bg-indigo-500/20 transition-all flex items-center gap-2"
				>
					<svg
						class="w-3.5 h-3.5"
						fill="none"
						viewBox="0 0 24 24"
						stroke="currentColor"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z"
						/>
					</svg>
					Assistant
				</button>
			</div>
			<div class="flex items-center gap-6">
				<div class="flex flex-col">
					<label
						for="target-date"
						class="text-[10px] font-bold text-gray-500 uppercase tracking-widest"
						>Target Date</label
					>
					<input
						id="target-date"
						type="date"
						bind:value={date}
						class="bg-transparent border-none text-sm font-semibold text-blue outline-none focus:text-orange-400 transition-colors"
					/>
				</div>
				<div class="h-8 w-px bg-white/10"></div>
				<button
					onclick={handleRun}
					class="px-5 py-2 bg-orange-500 hover:bg-orange-600 text-black text-xs font-bold rounded-lg transition-all active:scale-95 shadow-lg shadow-orange-500/20"
				>
					Run Analysis
				</button>
			</div>

			<div class="flex items-center gap-3">
				<button
					onclick={handleSend}
					disabled={!report_html}
					class="px-4 py-2 bg-white/5 border border-white/10 hover:bg-white/10 disabled:opacity-30 disabled:cursor-not-allowed rounded-lg text-xs font-bold transition-colors"
				>
					Send to Slack
				</button>
			</div>
		</header>

		<div class="flex-1 overflow-y-auto p-8">
			<div class="grid grid-cols-1 xl:grid-cols-2 gap-8 h-full">
				<!-- Left Column: Stages & Tasks -->
				<div class="flex flex-col gap-8">
					<section
						class="bg-[#0d1117] border border-white/10 rounded-2xl overflow-hidden shadow-xl"
					>
						<div
							class="px-6 py-4 border-b border-white/10 flex items-center justify-between"
						>
							<h3
								class="text-xs font-bold text-gray-400 uppercase tracking-widest"
							>
								Pipeline Stages
							</h3>
						</div>
						<div class="p-6 grid grid-cols-2 gap-4">
							{#each stages as stage}
								<div
									role="presentation"
									onmousemove={(e) => {
										const rect =
											e.currentTarget.getBoundingClientRect();
										e.currentTarget.style.setProperty(
											"--x",
											`${e.clientX - rect.left}px`,
										);
										e.currentTarget.style.setProperty(
											"--y",
											`${e.clientY - rect.top}px`,
										);
									}}
									class="relative flex items-center gap-3 p-3 rounded-xl bg-white/5 border border-white/5 overflow-hidden {stage.status ===
									'running'
										? 'animate-lightning'
										: ''} {stage.status === 'done'
										? 'border-green-500/20 bg-green-500/5'
										: ''}"
								>
									{#if stage.status === "running"}
										<div
											class="absolute inset-0 bg-green-500/5 pointer-events-none"
										></div>
										<div
											class="absolute top-0 left-0 w-full h-[1px] bg-gradient-to-r from-transparent via-green-400 to-transparent lightning-line"
										></div>
									{/if}

									<div
										class="z-10 flex shrink-0 items-center justify-center"
									>
										{#if stage.status === "done"}
											<div class="w-4 h-4 text-green-400">
												<svg
													fill="none"
													viewBox="0 0 24 24"
													stroke="currentColor"
												>
													<path
														stroke-linecap="round"
														stroke-linejoin="round"
														stroke-width="3"
														d="M5 13l4 4L19 7"
													/>
												</svg>
											</div>
										{:else}
											<div
												class="w-2 h-2 rounded-full {stage.status ===
												'running'
													? 'bg-orange-500 animate-pulse'
													: 'bg-gray-600'}"
											></div>
										{/if}
									</div>

									<div class="flex flex-col z-10">
										<span
											class="text-xs font-medium {stage.status ===
											'done'
												? 'text-green-100'
												: 'text-gray-200'}"
											>{stage.name}</span
										>
										{#if stage.duration}
											<span
												class="text-[10px] {stage.status ===
												'done'
													? 'text-green-500/70'
													: 'text-gray-500'}"
												>{stage.duration}</span
											>
										{/if}
									</div>
								</div>
							{/each}
						</div>
					</section>

					<section
						class="flex-1 bg-[#0d1117] border border-white/10 rounded-2xl overflow-hidden shadow-xl flex flex-col"
					>
						<div
							class="px-6 py-4 border-b border-white/10 flex items-center justify-between shrink-0"
						>
							<h3
								class="text-xs font-bold text-gray-400 uppercase tracking-widest"
							>
								Synthesized Tasks
							</h3>
							<div class="flex items-center gap-3">
								<button
									onclick={handleClearTasks}
									disabled={!tasks.length}
									class="text-[10px] font-bold text-red-400 hover:text-red-300 transition-colors uppercase tracking-tight disabled:opacity-30 disabled:cursor-not-allowed"
								>
									Clear All
								</button>
								<span
									class="px-2 py-0.5 rounded-full bg-orange-500/10 text-orange-400 text-[10px] font-bold"
									>{tasks.length} Total</span
								>
							</div>
						</div>
						<div class="flex-1 overflow-y-auto p-6">
							<TaskList {tasks} onTaskAction={handleTaskAction} />
						</div>
					</section>
				</div>

				<!-- Right Column: Preview -->

				<section
					class="bg-[#0d1117] border border-white/10 rounded-2xl overflow-hidden shadow-xl flex flex-col"
				>
					<div
						class="px-6 py-4 border-b border-white/10 flex items-center justify-between shrink-0"
					>
						<h3
							class="text-xs font-bold text-gray-400 uppercase tracking-widest"
						>
							Slack Preview
						</h3>
						<span class="text-[10px] text-gray-500">wysiwyg</span>
					</div>
					<div
						class="flex-1 overflow-y-auto bg-[#1a1d21] p-10 font-sans text-[15px] text-[#d1d2d3] leading-relaxed"
					>
						{#if report_html}
							<div class="slack-content">
								{@html report_html}
							</div>
						{:else}
							<div
								class="h-full flex flex-col items-center justify-center text-gray-600"
							>
								<span class="text-sm"
									>Report will appear here</span
								>
							</div>
						{/if}
					</div>
				</section>
			</div>
		</div>
	</main>
</div>

<style>
	:global(.slack-content b) {
		color: #fff;
		font-weight: 700;
	}
	:global(.slack-content code) {
		background: rgba(255, 166, 87, 0.1);
		color: #ffa657;
		padding: 2px 4px;
		border-radius: 4px;
		font-family: "IBM Plex Mono", monospace;
		font-size: 13px;
	}
	:global(.slack-content ul) {
		padding-left: 20px;
		list-style-type: disc;
		margin: 12px 0;
	}
	:global(.slack-content li) {
		margin-bottom: 6px;
	}

	/* Hide scrollbar but keep functionality */
	.overflow-y-auto {
		scrollbar-width: thin;
		scrollbar-color: #30363d transparent;
	}
	input[type="date"]::-webkit-calendar-picker-indicator {
		filter: invert(1); /* Torna o ícone branco em fundo escuro */
	}
	@keyframes lightning-flow {
		0% {
			transform: translateX(-100%);
			opacity: 0;
		}
		20% {
			opacity: 1;
		}
		80% {
			opacity: 1;
		}
		100% {
			transform: translateX(100%);
			opacity: 0;
		}
	}

	@keyframes electric-pulse {
		0%,
		100% {
			box-shadow: 0 0 5px rgba(74, 222, 128, 0.2);
			border-color: rgba(74, 222, 128, 0.1);
		}
		50% {
			box-shadow: 0 0 15px rgba(74, 222, 128, 0.4);
			border-color: rgba(74, 222, 128, 0.3);
		}
	}

	.animate-lightning {
		animation: electric-pulse 2s infinite ease-in-out;
	}

	.lightning-line {
		animation: lightning-flow 3s infinite linear;
	}

	.animate-lightning::after {
		content: "";
		position: absolute;
		inset: 0;
		background: radial-gradient(
			circle at var(--x, 50%) var(--y, 50%),
			rgba(74, 222, 128, 0.2) 0%,
			transparent 50%
		);
		opacity: 0;
		transition: opacity 0.3s;
		pointer-events: none;
	}

	.animate-lightning:hover::after {
		opacity: 1;
	}

	@keyframes electric-strike {
		0%,
		100% {
			opacity: 0;
		}
		5%,
		15% {
			opacity: 1;
			filter: brightness(2);
		}
		10% {
			opacity: 0.5;
		}
	}

	.animate-lightning {
		position: relative;
	}

	.animate-lightning::before {
		content: "";
		position: absolute;
		inset: 0;
		border: 1px solid #4ade80;
		border-radius: inherit;
		opacity: 0;
		animation: electric-strike 5s infinite;
		pointer-events: none;
	}
</style>
