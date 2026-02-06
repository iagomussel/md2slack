<script>
    /** @type {{ task: any, index: number, onClose: () => void, onSave: (index: number, task: any) => void }} */
    let { task, index, onClose, onSave } = $props();

    let intent = $state(task?.task_intent || "");
    let scope = $state(task?.scope || "");
    let type = $state(task?.task_type || "delivery");
    let estimated_hours = $state(task?.estimated_hours || 1);
    let details = $state(task?.technical_why || "");
    let commits = $state(task?.commits?.join(", ") || "");

    function save() {
        onSave(index, {
            ...task,
            task_intent: intent,
            scope: scope,
            task_type: type,
            estimated_hours: Number(estimated_hours),
            technical_why: details,
            commits: commits
                .split(",")
                .map((c) => c.trim())
                .filter((c) => c),
        });
        onClose();
    }
</script>

<div
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 backdrop-blur-sm animate-in fade-in duration-200"
>
    <div
        class="bg-[#0d1117] border border-white/10 rounded-xl w-full max-w-2xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]"
    >
        <div
            class="p-5 border-b border-white/10 flex items-center justify-between"
        >
            <h3 class="font-bold text-lg text-white">Edit Task #{index}</h3>
            <button
                onclick={onClose}
                aria-label="Close modal"
                class="text-gray-400 hover:text-white transition-colors"
            >
                <svg
                    class="w-5 h-5"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                >
                    <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M6 18L18 6M6 6l12 12"
                    />
                </svg>
            </button>
        </div>

        <div class="p-6 space-y-4 overflow-y-auto">
            <div class="grid grid-cols-2 gap-4">
                <div>
                    <label
                        for="task-intent"
                        class="block text-xs font-bold text-gray-500 uppercase mb-1"
                        >Intent</label
                    >
                    <input
                        id="task-intent"
                        bind:value={intent}
                        class="w-full bg-[#1c2128] border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:border-blue-500/50 outline-none"
                    />
                </div>
                <div>
                    <label
                        for="task-scope"
                        class="block text-xs font-bold text-gray-500 uppercase mb-1"
                        >Scope</label
                    >
                    <input
                        id="task-scope"
                        bind:value={scope}
                        class="w-full bg-[#1c2128] border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:border-blue-500/50 outline-none"
                    />
                </div>
            </div>

            <div class="grid grid-cols-2 gap-4">
                <div>
                    <label
                        for="task-type"
                        class="block text-xs font-bold text-gray-500 uppercase mb-1"
                        >Type</label
                    >
                    <select
                        id="task-type"
                        bind:value={type}
                        class="w-full bg-[#1c2128] border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:border-blue-500/50 outline-none"
                    >
                        <option value="delivery">Delivery</option>
                        <option value="fix">Fix</option>
                        <option value="chore">Chore</option>
                        <option value="refactor">Refactor</option>
                    </select>
                </div>
                <div>
                    <label
                        for="task-hours"
                        class="block text-xs font-bold text-gray-500 uppercase mb-1"
                        >Est. Hours</label
                    >
                    <input
                        id="task-hours"
                        type="number"
                        bind:value={estimated_hours}
                        class="w-full bg-[#1c2128] border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:border-blue-500/50 outline-none"
                    />
                </div>
            </div>

            <div>
                <label
                    for="task-details"
                    class="block text-xs font-bold text-gray-500 uppercase mb-1"
                    >Technical Details (Bullets)</label
                >
                <textarea
                    id="task-details"
                    bind:value={details}
                    class="w-full bg-[#1c2128] border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:border-blue-500/50 outline-none h-32 resize-none"
                ></textarea>
            </div>

            <div>
                <label
                    for="task-commits"
                    class="block text-xs font-bold text-gray-500 uppercase mb-1"
                    >Commits (Comma Separated)</label
                >
                <input
                    id="task-commits"
                    bind:value={commits}
                    class="w-full bg-[#1c2128] border border-white/10 rounded-lg px-3 py-2 text-sm text-gray-400 focus:border-blue-500/50 outline-none font-mono"
                />
            </div>
        </div>

        <div
            class="p-5 border-t border-white/10 bg-[#161b22] flex justify-end gap-3"
        >
            <button
                onclick={onClose}
                class="px-4 py-2 text-sm font-medium text-gray-400 hover:text-white transition-colors"
            >
                Cancel
            </button>
            <button
                onclick={save}
                class="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-500 transition-colors shadow-lg shadow-blue-500/20"
            >
                Save Changes
            </button>
        </div>
    </div>
</div>
