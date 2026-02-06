<script>
	import GitGraph from "./GitGraph.svelte";

	let {
		appState = {},
		projects = [],
		settings = {},
		onSelectProject,
		onSelectUser,
		onAddProject,
		onScanUsers,
		onCommitClick,
	} = $props();

	/** @type {any[]} */
	let commits = $state([]);

	/** @param {string} path */
	async function loadGraph(path) {
		if (!path) return;
		try {
			const res = await fetch(
				`/api/git-graph?path=${encodeURIComponent(path)}`,
			);
			if (res.ok) {
				const data = await res.json();
				commits = data.commits || [];
			}
		} catch (e) {
			console.error("Failed to load graph", e);
		}
	}

	$effect(() => {
		loadGraph(appState.selectedProject);
	});
</script>

<aside
	class="flex flex-col w-80 border-r border-white/10 bg-[#0d1117] h-screen overflow-hidden"
>
	<!-- Workspace Header -->
	<div class="p-6 border-b border-white/10">
		<div class="flex items-center gap-3 mb-6">
			<div
				class="w-8 h-8 rounded-lg bg-gradient-to-br from-orange-500 to-orange-600 shadow-lg shadow-orange-500/20 flex items-center justify-center"
			>
				<div class="w-4 h-4 rounded-sm border-2 border-white/80"></div>
			</div>
			<h1 class="text-lg font-bold tracking-tight text-white">ssbot</h1>
		</div>

		<h2
			class="text-[10px] font-bold uppercase tracking-widest text-gray-500 mb-3"
		>
			Workspace
		</h2>

		<div class="space-y-4">
			<div>
				<label
					for="project-select"
					class="block text-[10px] font-bold text-gray-600 uppercase mb-1.5 ml-1"
					>Project</label
				>
				<select
					id="project-select"
					class="w-full bg-black/20 border border-white/10 rounded-lg px-3 py-2 text-xs text-gray-200 outline-none focus:border-orange-500/50 hover:bg-black/30 transition-colors"
					value={appState.selectedProject}
					onchange={(e) => {
						const target = /** @type {HTMLSelectElement} */ (
							e.target
						);
						if (target) onSelectProject(target.value);
					}}
				>
					{#each projects as p}
						<option value={p.path}>{p.name}</option>
					{/each}
					{#if projects.length === 0}
						<option value="">(no projects)</option>
					{/if}
				</select>
			</div>

			<div class="flex gap-2">
				<button
					onclick={onAddProject}
					class="flex-1 px-3 py-2 bg-white/5 border border-white/10 rounded-lg text-[10px] font-bold text-gray-300 hover:bg-white/10 hover:text-white transition-all active:scale-95"
				>
					Add Path
				</button>
				<button
					onclick={onScanUsers}
					class="flex-1 px-3 py-2 bg-white/5 border border-white/10 rounded-lg text-[10px] font-bold text-gray-300 hover:bg-white/10 hover:text-white transition-all active:scale-95"
				>
					Scan Users
				</button>
			</div>

			<div>
				<label
					for="author-select"
					class="block text-[10px] font-bold text-gray-600 uppercase mb-1.5 ml-1"
					>Authors (Hold Cmd/Ctrl)</label
				>
				<select
					id="author-select"
					multiple
					class="w-full bg-black/20 border border-white/10 rounded-lg px-3 py-2 text-xs text-gray-200 outline-none focus:border-orange-500/50 hover:bg-black/30 transition-colors h-24"
					value={appState.selectedUser
						? appState.selectedUser.split(",")
						: []}
					onchange={(e) => {
						const target = /** @type {HTMLSelectElement} */ (
							e.target
						);
						const selected = Array.from(target.selectedOptions).map(
							(o) => o.value,
						);
						onSelectUser(selected.join(","));
					}}
				>
					{#each settings.usernames || [] as u}
						<option value={u}>{u}</option>
					{/each}
					{#if !settings.usernames || settings.usernames.length === 0}
						<option value="">(no users)</option>
					{/if}
				</select>
			</div>
		</div>
	</div>

	<!-- Commits Graph Section -->
	<div class="flex-1 flex flex-col min-h-0">
		<div class="px-6 py-4 flex items-center justify-between">
			<h2
				class="text-[10px] font-bold uppercase tracking-widest text-gray-500"
			>
				Commits History
			</h2>
			<div class="flex gap-1">
				<div
					class="w-1.5 h-1.5 rounded-full bg-green-500 animate-pulse"
				></div>
			</div>
		</div>
		<div class="flex-1 overflow-y-auto">
			<GitGraph {commits} {onCommitClick} />
		</div>
	</div>

	<!-- System Logs -->
	<div class="p-5 border-t border-white/10 bg-black/20">
		<h2
			class="text-[10px] font-bold uppercase tracking-widest text-gray-500 mb-2 flex items-center gap-2"
		>
			Terminal
			<span
				class="px-1.5 py-0.5 rounded bg-white/5 text-[9px] lowercase font-medium"
				>live</span
			>
		</h2>
		<div
			class="bg-black/40 rounded-lg p-3 font-mono text-[10px] text-gray-400 h-32 overflow-y-auto whitespace-pre-wrap border border-white/5"
		>
			{appState.logs?.join("\n") || "Initializing system..."}
		</div>
	</div>
</aside>
