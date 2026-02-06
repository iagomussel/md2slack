<script>
    /** @type {{ isOpen: boolean, onClose: () => void, onTasksUpdate: (tasks: any[]) => void }} */
    let { isOpen, onClose, onTasksUpdate } = $props();

    /** @type {any[]} */
    let messages = $state([]);
    let input = $state("");
    let loading = $state(false);

    /** @type {HTMLDivElement|undefined} */
    let chatContainer = $state();

    $effect(() => {
        if (isOpen && chatContainer) {
            chatContainer.scrollTop = chatContainer.scrollHeight;
        }
    });

    async function sendMessage() {
        if (!input.trim() || loading) return;

        const userContent = input;
        input = "";
        loading = true;

        // Add user message
        messages = [
            ...messages,
            { role: "user", content: userContent, type: "text" },
        ];

        // Add placeholder assistant message
        messages = [
            ...messages,
            {
                role: "assistant",
                content: "",
                type: "streaming",
                tools: [],
            },
        ];

        const assistantIndex = messages.length - 1;

        const history = messages
            .filter((m) => m.type === "text")
            .map((m) => ({ role: m.role, content: m.content }));

        try {
            const res = await fetch("/api/chat", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ history }),
            });

            if (!res.ok) {
                throw new Error("Chat request failed");
            }

            const reader = res.body?.getReader();
            const decoder = new TextDecoder();

            if (!reader) {
                throw new Error("No response body");
            }

            let buffer = "";
            let currentEvent = "";

            while (true) {
                const { done, value } = await reader.read();
                if (done) break;

                buffer += decoder.decode(value, { stream: true });
                const lines = buffer.split("\n");
                buffer = lines.pop() || "";

                for (const line of lines) {
                    if (!line.trim()) {
                        currentEvent = ""; // Reset on empty line (end of event)
                        continue;
                    }

                    if (line.startsWith("event:")) {
                        currentEvent = line.substring(6).trim();
                    } else if (line.startsWith("data:")) {
                        const dataStr = line.substring(5).trim();
                        try {
                            const data = JSON.parse(dataStr);

                            if (
                                currentEvent === "message" ||
                                data.text !== undefined
                            ) {
                                messages[assistantIndex].content =
                                    data.text || "";
                                messages[assistantIndex].type = "text";
                                messages = [...messages];
                                if (data.tasks) {
                                    onTasksUpdate(data.tasks);
                                }
                            } else if (currentEvent === "chunk") {
                                messages[assistantIndex].content +=
                                    data.text || "";
                                messages[assistantIndex].type = "streaming";
                                messages = [...messages];
                            } else if (currentEvent === "tool_start") {
                                // Add tool to the assistant message
                                if (!messages[assistantIndex].tools) {
                                    messages[assistantIndex].tools = [];
                                }
                                messages[assistantIndex].tools.push({
                                    name: data.tool,
                                    params: data.params,
                                    result: null,
                                });
                                messages = [...messages];
                            } else if (currentEvent === "tool_end") {
                                // Update the tool with result
                                const tool = messages[
                                    assistantIndex
                                ].tools?.find(
                                    (t) => t.name === data.tool && !t.result,
                                );
                                if (tool) {
                                    tool.result = data.result;
                                    messages = [...messages];
                                }
                            } else if (
                                currentEvent === "error" ||
                                data.message
                            ) {
                                messages[assistantIndex].content =
                                    `Error: ${data.message}`;
                                messages[assistantIndex].type = "text";
                                messages = [...messages];
                            } else if (
                                currentEvent === "done" ||
                                data.status === "complete"
                            ) {
                                loading = false;
                            }
                        } catch (e) {
                            console.error(
                                "Failed to parse SSE data:",
                                e,
                                dataStr,
                            );
                        }
                    }
                }
            }
        } catch (e) {
            console.error("Chat error:", e);
            messages[assistantIndex].content = "Sorry, an error occurred.";
            messages[assistantIndex].type = "text";
            messages = [...messages];
        } finally {
            loading = false;
            setTimeout(() => {
                if (chatContainer) {
                    chatContainer.scrollTop = chatContainer.scrollHeight;
                }
            }, 50);
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
            {#if messages.length === 0}
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

            {#each messages as msg}
                <div
                    class="flex {msg.role === 'user'
                        ? 'justify-end'
                        : 'justify-start'}"
                >
                    <div
                        class="max-w-[85%] rounded-lg px-4 py-2 {msg.role ===
                        'user'
                            ? 'bg-orange-600 text-white'
                            : 'bg-white/5 text-gray-200'}"
                    >
                        <div class="text-sm whitespace-pre-wrap">
                            {msg.content}
                        </div>

                        {#if msg.tools && msg.tools.length > 0}
                            <div class="mt-2 space-y-2">
                                {#each msg.tools as tool}
                                    <div
                                        class="bg-black/20 rounded border border-white/10 p-2"
                                    >
                                        <div
                                            class="text-xs font-mono text-orange-400"
                                        >
                                            🔧 {tool.name}
                                        </div>
                                        {#if tool.params}
                                            <details class="mt-1">
                                                <summary
                                                    class="text-xs text-gray-400 cursor-pointer hover:text-gray-300"
                                                >
                                                    Parameters
                                                </summary>
                                                <pre
                                                    class="text-xs text-gray-500 mt-1 overflow-x-auto">{JSON.stringify(
                                                        tool.params,
                                                        null,
                                                        2,
                                                    )}</pre>
                                            </details>
                                        {/if}
                                        {#if tool.result}
                                            <details class="mt-1">
                                                <summary
                                                    class="text-xs text-gray-400 cursor-pointer hover:text-gray-300"
                                                >
                                                    Result
                                                </summary>
                                                <pre
                                                    class="text-xs text-gray-500 mt-1 overflow-x-auto">{tool.result}</pre>
                                            </details>
                                        {/if}
                                    </div>
                                {/each}
                            </div>
                        {/if}
                    </div>
                </div>
            {/each}

            {#if loading}
                <div class="flex justify-start">
                    <div
                        class="bg-white/5 text-gray-400 rounded-lg px-4 py-2 text-sm"
                    >
                        <div class="flex items-center gap-2">
                            <div
                                class="w-2 h-2 bg-orange-500 rounded-full animate-pulse"
                            ></div>
                            Thinking...
                        </div>
                    </div>
                </div>
            {/if}
        </div>

        <div class="p-4 border-t border-white/10">
            <form
                onsubmit={(e) => {
                    e.preventDefault();
                    sendMessage();
                }}
                class="flex gap-2"
            >
                <input
                    bind:value={input}
                    disabled={loading}
                    placeholder="Ask me anything..."
                    class="flex-1 bg-black/20 border border-white/10 rounded-lg px-3 py-2 text-sm text-gray-200 placeholder-gray-500 outline-none focus:border-orange-500/50 disabled:opacity-50"
                />
                <button
                    type="submit"
                    disabled={loading || !input.trim()}
                    class="px-4 py-2 bg-orange-600 hover:bg-orange-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white rounded-lg text-sm font-medium transition-colors"
                >
                    Send
                </button>
            </form>
        </div>
    </div>
{/if}
