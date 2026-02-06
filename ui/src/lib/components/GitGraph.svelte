<script>
	/** @type {{ commits: any[], onCommitClick?: (commit: any) => void }} */
	let { commits = [], onCommitClick } = $props();

	const COLORS = [
		"#3b82f6", // blue
		"#10b981", // green
		"#f59e0b", // amber
		"#ef4444", // red
		"#8b5cf6", // violet
		"#ec4899", // pink
		"#06b6d4", // cyan
	];

	// Graph Layout logic
	let graphData = $derived.by(() => {
		if (!commits || commits.length === 0) return [];

		/** @type {Map<string, number>} */
		const commitToTrack = new Map();
		/** @type {(string|null)[]} */
		let activeTracks = [];

		return commits.map((c, i) => {
			let trackIndex = activeTracks.indexOf(c.hash);

			if (trackIndex === -1) {
				trackIndex = activeTracks.findIndex((t) => t === null);
				if (trackIndex === -1) {
					trackIndex = activeTracks.length;
					activeTracks.push(null);
				}
			}

			// Save connections for rendering lines
			const connections = [];
			const currentTrack = trackIndex;

			// Update active tracks with parents
			if (c.parents && c.parents.length > 0) {
				// Primary parent stays on same track
				activeTracks[trackIndex] = c.parents[0];
				connections.push({
					from: currentTrack,
					to: currentTrack,
					type: "vertical",
				});

				// Secondary parents (merges)
				for (let j = 1; j < c.parents.length; j++) {
					// Merges come from another track.
					// Simplified: just show it coming from the next available
				}
			} else {
				activeTracks[trackIndex] = null;
			}

			return {
				...c,
				track: trackIndex,
				color: COLORS[trackIndex % COLORS.length],
			};
		});
	});
</script>

<div class="flex flex-col space-y-0.5 font-mono text-[11px] select-none py-2">
	{#each graphData as c, i}
		<button
			type="button"
			onclick={() => onCommitClick?.(c)}
			class="group flex items-center h-10 w-full hover:bg-white/5 border-l-2 border-transparent hover:border-orange-500/50 transition-all text-left"
		>
			<!-- Graph Column -->
			<div class="relative w-16 h-full flex shrink-0">
				<!-- Vertical Line -->
				<div
					class="absolute top-0 bottom-0 w-0.5 opacity-40 transition-opacity group-hover:opacity-100"
					style="background: {c.color}; left: {c.track * 12 + 24}px;"
				></div>

				<!-- Commit Node (Circle) -->
				<div
					class="absolute top-1/2 -translate-y-1/2 w-3 h-3 rounded-full border-2 border-[#0d1117] z-10
						   shadow-[0_0_10px_rgba(0,0,0,0.5)] transition-transform group-hover:scale-125"
					style="background: {c.color}; left: {c.track * 12 +
						21}px; box-shadow: 0 0 10px {c.color}66;"
				>
					{#if i === 0}
						<div
							class="absolute inset-0 rounded-full animate-ping bg-current opacity-20"
							style="color: {c.color}"
						></div>
					{/if}
				</div>
			</div>

			<!-- Content -->
			<div class="flex-1 flex flex-col justify-center min-w-0 pr-4">
				<div class="flex items-center gap-2 overflow-hidden">
					<span
						class="text-gray-200 font-semibold truncate shrink leading-tight group-hover:text-white transition-colors"
						>{c.subject}</span
					>
					<div class="flex gap-1 shrink-0">
						{#each c.refs as ref}
							<span
								class="px-1.5 py-0.5 rounded bg-blue-500/10 border border-blue-500/20 text-[9px] font-bold text-blue-400 whitespace-nowrap"
							>
								{ref}
							</span>
						{/each}
					</div>
				</div>
				<div
					class="flex items-center gap-2 text-gray-500 text-[10px] mt-0.5"
				>
					<span class="text-gray-600 font-bold"
						>{c.hash.substring(0, 7)}</span
					>
					<span class="opacity-30">•</span>
					<span class="truncate opacity-80">{c.author}</span>
				</div>
			</div>
		</button>
	{/each}
</div>

<style>
	/* Subtle hide scrollbar */
	div::-webkit-scrollbar {
		width: 3px;
	}
	div::-webkit-scrollbar-thumb {
		background: #21262d;
		border-radius: 10px;
	}
	div::-webkit-scrollbar-track {
		background: transparent;
	}
</style>
