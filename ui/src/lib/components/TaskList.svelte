<script>
	/** @type {{ tasks: any[], onTaskAction?: (index: number, action: string) => void }} */
	let { tasks = [], onTaskAction } = $props();

	let openMenuIndex = $state(-1);

	/** @param {string} status */
	function getStatusStyles(status = "done") {
		switch (status) {
			case "in_progress":
				return "text-blue-400 bg-blue-500/10 border-blue-500/20";
			case "blocked":
				return "text-red-400 bg-red-500/10 border-red-500/20";
			default:
				return "text-green-400 bg-green-500/10 border-green-500/20";
		}
	}

	const ACTIONS = [
		{
			id: "make_longer",
			label: "More Detail",
			icon: "M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4",
		},
		{
			id: "make_shorter",
			label: "Shorten",
			icon: "M4 4l5 5m11-5l-5 5M4 20l5-5m11 5l-5-5",
		},
		{
			id: "improve_text",
			label: "Improve Text",
			icon: "M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z",
		},
		{
			id: "split_task",
			label: "Split Task",
			icon: "M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4",
		},
	];

	function closeMenu() {
		openMenuIndex = -1;
	}
</script>

<svelte:window onclick={closeMenu} />

<div class="flex flex-col gap-4">
	{#each tasks as task}
		<div
			class="p-5 rounded-2xl bg-[#161b22] border border-white/5 hover:border-white/20 hover:bg-[#1c2128] transition-all cursor-pointer group shadow-lg"
		>
			<div
				class="flex items-start justify-between mb-3 overflow-visible gap-4 relative"
			>
				<h4
					class="text-sm font-bold text-gray-100 group-hover:text-orange-400 transition-colors leading-snug flex-1"
				>
					{task.task_intent}
				</h4>

				<div class="flex items-center gap-2 shrink-0">
					<span
						class="px-2.5 py-0.5 rounded-full text-[9px] font-black uppercase tracking-widest border shrink-0 {getStatusStyles(
							task.status,
						)}"
					>
						{task.status || "done"}
					</span>

					<div class="relative">
						<button
							onclick={(e) => {
								e.stopPropagation();
								openMenuIndex =
									openMenuIndex === tasks.indexOf(task)
										? -1
										: tasks.indexOf(task);
							}}
							aria-label="Task Actions"
							class="p-1 hover:bg-white/10 rounded transition-colors text-gray-500 hover:text-white"
						>
							<svg
								class="w-4 h-4"
								fill="currentColor"
								viewBox="0 0 20 20"
							>
								<path
									d="M6 10a2 2 0 11-4 0 2 2 0 014 0zM12 10a2 2 0 11-4 0 2 2 0 014 0zM18 10a2 2 0 11-4 0 2 2 0 014 0z"
								/>
							</svg>
						</button>

						{#if openMenuIndex === tasks.indexOf(task)}
							<div
								class="absolute right-0 mt-1 w-48 bg-[#1c2128] border border-white/10 rounded-xl shadow-2xl z-50 overflow-hidden py-1 animate-in fade-in slide-in-from-top-2 duration-200"
							>
								{#each ACTIONS as action}
									<button
										onclick={(e) => {
											e.stopPropagation();
											onTaskAction?.(
												tasks.indexOf(task),
												action.id,
											);
											closeMenu();
										}}
										class="w-full flex items-center gap-3 px-4 py-2.5 text-xs text-gray-300 hover:bg-white/5 hover:text-orange-400 transition-colors text-left"
									>
										<svg
											class="w-3.5 h-3.5 opacity-60"
											fill="none"
											viewBox="0 0 24 24"
											stroke="currentColor"
										>
											<path
												stroke-linecap="round"
												stroke-linejoin="round"
												stroke-width="2"
												d={action.icon}
											/>
										</svg>
										{action.label}
									</button>
								{/each}
							</div>
						{/if}
					</div>
				</div>
			</div>

			{#if task.technical_why}
				<p
					class="text-[12px] text-gray-400 line-clamp-3 leading-relaxed mb-4 font-medium opacity-80"
				>
					{task.technical_why}
				</p>
			{/if}

			<div class="flex items-center gap-4 pt-4 border-t border-white/5">
				<div
					class="flex items-center gap-1.5 text-[10px] font-bold text-gray-500"
				>
					<svg
						class="w-3 h-3 opacity-40"
						fill="none"
						viewBox="0 0 24 24"
						stroke="currentColor"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
						/>
					</svg>
					<span>{task.estimated_hours || 1}h</span>
				</div>
				{#if task.scope}
					<div
						class="flex items-center gap-1.5 text-[10px] font-bold text-gray-500 min-w-0"
					>
						<svg
							class="w-3 h-3 opacity-40 shrink-0"
							fill="none"
							viewBox="0 0 24 24"
							stroke="currentColor"
						>
							<path
								stroke-linecap="round"
								stroke-linejoin="round"
								stroke-width="2"
								d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z"
							/>
						</svg>
						<span class="truncate">{task.scope}</span>
					</div>
				{/if}
				{#if task.task_type}
					<div
						class="ml-auto px-2 py-0.5 rounded bg-white/5 text-[9px] font-bold text-gray-600 uppercase tracking-tighter"
					>
						{task.task_type}
					</div>
				{/if}
			</div>
		</div>
	{/each}

	{#if tasks.length === 0}
		<div
			class="h-64 flex flex-col items-center justify-center text-gray-600 border-2 border-dashed border-white/5 rounded-3xl"
		>
			<svg
				class="w-10 h-10 mb-3 opacity-20"
				fill="none"
				viewBox="0 0 24 24"
				stroke="currentColor"
			>
				<path
					stroke-linecap="round"
					stroke-linejoin="round"
					stroke-width="1.5"
					d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
				/>
			</svg>
			<span class="text-sm font-medium">Capture commits to see tasks</span
			>
		</div>
	{/if}
</div>
