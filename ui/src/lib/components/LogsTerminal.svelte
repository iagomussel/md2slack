<script>
	let { appState } = $props();
</script>


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
        <pre>{appState.logs?.join("\n") || "Initializing system..."}</pre>
    </div>
</div>
<style>
    
</style>