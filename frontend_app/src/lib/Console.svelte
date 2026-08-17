<script>
  import { onDestroy } from "svelte";
  import { consoleTicket } from "./api.js";

  let open = $state(false);
  let lines = $state([]);
  let err = $state("");
  let ws = null;
  let box = $state();

  function wsUrl(ticket) {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    let p = location.pathname;
    if (p.endsWith("/")) p = p.slice(0, -1);
    return `${proto}//${location.host}${p}/api/console/ws?ticket=${encodeURIComponent(ticket)}`;
  }

  async function openConsole() {
    err = "";
    lines = [];
    try {
      const ticket = await consoleTicket();
      ws = new WebSocket(wsUrl(ticket));
      ws.onopen = () => {
        open = true;
      };
      ws.onmessage = (ev) => {
        lines = [...lines, ev.data].slice(-500);
        queueMicrotask(() => {
          if (box) box.scrollTop = box.scrollHeight;
        });
      };
      ws.onerror = () => {
        err = "console connection error";
      };
      ws.onclose = () => {
        open = false;
        ws = null;
      };
    } catch (e) {
      err = e.message || String(e);
    }
  }

  function closeConsole() {
    if (ws) ws.close();
    ws = null;
    open = false;
  }

  onDestroy(() => {
    if (ws) ws.close();
  });
</script>

<div class="console">
  <div class="controls">
    {#if open}
      <button onclick={closeConsole}>Close console</button>
      <span class="hint">tailing service journal — closing stops it</span>
    {:else}
      <button onclick={openConsole}>Open console</button>
      <span class="hint">logs are read only while the console is open</span>
    {/if}
  </div>

  {#if err}
    <div class="err">{err}</div>
  {/if}

  {#if open}
    <pre class="box" bind:this={box}>{lines.join("\n")}</pre>
  {/if}
</div>

<style>
  .controls {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.5rem;
  }
  .hint {
    font-size: 0.8rem;
    opacity: 0.6;
  }
  .err {
    color: #ff6b6b;
    font-size: 0.85rem;
    margin-bottom: 0.5rem;
  }
  .box {
    background: #101014;
    color: #d0d0d0;
    padding: 0.75rem;
    border-radius: 8px;
    height: 320px;
    overflow: auto;
    white-space: pre-wrap;
    word-break: break-word;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.8rem;
    margin: 0;
  }
</style>
