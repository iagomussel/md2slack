<script>
	/** @type {{ tasks: any[] }} */
	let { tasks = [] } = $props();

	/** @param {string} status */
	function getStatusStyles(status = 'done') {
		switch (status) {
			case 'in_progress': return 'text-blue-400 bg-blue-500/10 border-blue-500/20';
			case 'blocked': return 'text-red-400 bg-red-500/10 border-red-500/20';
			default: return 'text-green-400 bg-green-500/10 border-green-500/20';
		}
	}
</script>

<div class="flex flex-col gap-4">
	{#each tasks as task}
		<div class="p-5 rounded-2xl bg-[#161b22] border border-white/5 hover:border-white/20 hover:bg-[#1c2128] transition-all cursor-pointer group shadow-lg">
			<div class="flex items-start justify-between mb-3 overflow-hidden gap-4">
				<h4 class="text-sm font-bold text-gray-100 group-hover:text-orange-400 transition-colors leading-snug">
					{task.task_intent}
				</h4>
				<span class="px-2.5 py-0.5 rounded-full text-[9px] font-black uppercase tracking-widest border shrink-0 {getStatusStyles(task.status)}">
					{task.status || 'done'}
				</span>
			</div>
			
			{#if task.technical_why}
				<p class="text-[12px] text-gray-400 line-clamp-3 leading-relaxed mb-4 font-medium opacity-80">
					{task.technical_why}
				</p>
			{/if}

			<div class="flex items-center gap-4 pt-4 border-t border-white/5">
				<div class="flex items-center gap-1.5 text-[10px] font-bold text-gray-500">
					<svg class="w-3 h-3 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
					<span>{task.estimated_hours || 1}h</span>
				</div>
				{#if task.scope}
					<div class="flex items-center gap-1.5 text-[10px] font-bold text-gray-500 min-w-0">
						<svg class="w-3 h-3 opacity-40 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
						</svg>
						<span class="truncate">{task.scope}</span>
					</div>
				{/if}
				{#if task.task_type}
					<div class="ml-auto px-2 py-0.5 rounded bg-white/5 text-[9px] font-bold text-gray-600 uppercase tracking-tighter">
						{task.task_type}
					</div>
				{/if}
			</div>
		</div>
	{/each}
	
	{#if tasks.length === 0}
		<div class="h-64 flex flex-col items-center justify-center text-gray-600 border-2 border-dashed border-white/5 rounded-3xl">
			<svg class="w-10 h-10 mb-3 opacity-20" fill="none" viewBox="0 0 24 24" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
			</svg>
			<span class="text-sm font-medium">Capture commits to see tasks</span>
		</div>
	{/if}
</div>
