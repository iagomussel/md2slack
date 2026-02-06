<script>
    /** @type {{ isOpen: boolean, onClose: () => void, onTasksUpdate: (tasks: any[]) => void }} */
    let { isOpen, onClose, onTasksUpdate } = $props();

    /** @type {any[]} */
    let history = $state([]);
    let input = $state("");
    let loading = $state(false);

    // Add scroll ref
    /** @type {HTMLDivElement|undefined} */
    let chatContainer = $state();

    $effect(() => {
        if (isOpen && chatContainer) {
            chatContainer.scrollTop = chatContainer.scrollHeight;
        }
    });

    async function sendMessage() {
        if (!input.trim() || loading) return;

        const userMsg = { role: "user", content: input };
        history = [...history, userMsg];
        input = "";
        loading = true;

        try {
            const res = await fetch("/api/chat", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ history: history }),
            });
            if (res.ok) {
                const data = await res.json();
                const assistantMsg = data.message;
                history = [...history, assistantMsg];
                if (data.tasks) {
                    onTasksUpdate(data.tasks);
                }
            } else {
                console.error("Chat failed");
            }
        } catch (e) {
            console.error("Chat error", e);
        } finally {
            loading = false;
            // Scroll to bottom
            if (chatContainer) {
                setTimeout(() => {
                    const el = /** @type {HTMLDivElement} */ (chatContainer);
                    el.scrollTop = el.scrollHeight;
                }, 50);
            }
        }
    }
</script>

{#if isOpen}
    <div
        class="fixed inset-y-0 right-0 w-96 bg-[#0d1117] border-l border-white/10 shadow-2xl z-50 flex flex-col animate-in slide-in-from-right duration-200"
    >
        <div
            class="p-4 border-b border-white/10 flex items-center justify-between"
        >
            <h3 class="font-bold text-gray-200">Assistant</h3>
            <button
                onclick={onClose}
                aria-label="Close assistant"
                class="p-1 hover:bg-white/10 rounded text-gray-400 hover:text-white"
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

        <div
            class="flex-1 overflow-y-auto p-4 space-y-4"
            bind:this={chatContainer}
        >
            {#if history.length === 0}
                <div class="text-center text-gray-500 text-sm mt-10">
                    <p>
                        Ask me to refine tasks, split items, or answer
                        questions.
                    </p>
                    <p class="text-xs mt-2 opacity-50">
                        e.g., "Combine tasks 1 and 2" or "Make intent more
                        formal"
                    </p>
                </div>
            {/if}
            {#each history as msg}
                <div
                    class="flex {msg.role === 'user'
                        ? 'justify-end'
                        : 'justify-start'}"
                >
                    <div
                        class="max-w-[85%] rounded-lg px-3 py-2 text-sm {msg.role ===
                        'user'
                            ? 'bg-blue-600 text-white'
                            : 'bg-[#1c2128] text-gray-300 border border-white/10'}"
                    >
                        <p class="whitespace-pre-wrap">{msg.content}</p>
                    </div>
                </div>
            {/each}
            {#if loading}
                <div class="flex justify-start">
                    <div
                        class="bg-[#1c2128] border border-white/10 rounded-lg px-3 py-2 text-sm text-gray-500 animate-pulse"
                    >
                        Thinking...
                    </div>
                </div>
            {/if}
        </div>

        <div class="p-4 border-t border-white/10 bg-[#161b22]">
            <div class="relative">
                <textarea
                    bind:value={input}
                    onkeydown={(e) => {
                        if (e.key === "Enter" && !e.shiftKey) {
                            e.preventDefault();
                            sendMessage();
                        }
                    }}
                    placeholder="Type a command..."
                    class="w-full bg-[#0d1117] border border-white/10 rounded-lg pl-3 pr-10 py-2 text-sm text-gray-200 outline-none focus:border-blue-500/50 resize-none h-20"
                ></textarea>
                <button
                    onclick={sendMessage}
                    aria-label="Send message"
                    disabled={loading || !input.trim()}
                    class="absolute bottom-2 right-2 p-1.5 bg-blue-600 text-white rounded-md hover:bg-blue-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                    <svg
                        class="w-4 h-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                    >
                        <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8"
                        />
                    </svg>
                </button>
            </div>
        </div>
    </div>
{/if}
