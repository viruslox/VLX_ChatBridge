<script>
  import { onMount, onDestroy } from "svelte";
  import { getStatus, toggleModule, toggleFeature, shutdown } from "./lib/api.js";
  import Console from "./lib/Console.svelte";

  let status = $state({ modules: [], features: [], restart_pending: false });
  let error = $state("");
  let busy = $state(false);
  let poll;

  async function refresh() {
    try {
      status = await getStatus();
      error = "";
    } catch (e) {
      error = e.message || String(e);
    }
  }

  onMount(() => {
    refresh();
    poll = setInterval(refresh, 3000);
  });
  onDestroy(() => clearInterval(poll));

  async function onModule(m) {
    busy = true;
    try {
      await toggleModule(m.name, !m.enabled_in_settings);
      await refresh();
    } catch (e) {
      error = e.message || String(e);
    }
    busy = false;
  }

  async function onFeature(f) {
    busy = true;
    try {
      await toggleFeature(f.key, !f.state);
      await refresh();
    } catch (e) {
      error = e.message || String(e);
    }
    busy = false;
  }

  async function onShutdown() {
    if (!confirm("Restart the ChatBridge service now to apply pending changes?")) return;
    busy = true;
    try {
      await shutdown();
      error = "";
    } catch (e) {
      error = e.message || String(e);
    }
    busy = false;
  }
</script>

<main>
  <h1>VLX ChatBridge</h1>

  {#if error}
    <div class="banner err">{error}</div>
  {/if}

  {#if status.restart_pending}
    <div class="banner warn">
      <span>Pending changes — running state differs from the settings file.</span>
      <button onclick={onShutdown} disabled={busy}>Apply &amp; restart</button>
    </div>
  {/if}

  <section class="card">
    <h2>Modules</h2>
    <div class="rows">
      {#each status.modules as m (m.name)}
        <div class="row">
          <span class="dot" class:on={m.running} title={m.running ? "running" : "stopped"}></span>
          <span class="name">{m.name}</span>
          <span class="meta">
            settings: {m.enabled_in_settings ? "enabled" : "disabled"}
            {#if m.running !== m.enabled_in_settings}<em>(restart to apply)</em>{/if}
          </span>
          <button
            class="toggle"
            class:active={m.enabled_in_settings}
            disabled={busy || !m.toggleable}
            onclick={() => onModule(m)}
          >
            {m.enabled_in_settings ? "ON" : "OFF"}
          </button>
        </div>
      {/each}
    </div>
  </section>

  <section class="card">
    <h2>Submodules</h2>
    <div class="rows">
      {#each status.features as f (f.key)}
        <div class="row">
          <span class="name">{f.label}</span>
          <span class="key">{f.key}</span>
          <button
            class="toggle"
            class:active={f.state}
            disabled={busy || !f.toggleable}
            onclick={() => onFeature(f)}
          >
            {f.state ? "ON" : "OFF"}
          </button>
        </div>
      {/each}
    </div>
  </section>

  <section class="card">
    <h2>Console</h2>
    <Console />
  </section>

  <section class="card danger">
    <h2>Service</h2>
    <button class="danger-btn" onclick={onShutdown} disabled={busy}>
      Shutdown (systemd relaunches)
    </button>
  </section>
</main>

<style>
  main {
    max-width: 960px;
    margin: 0 auto;
    padding: 2rem 1rem;
  }
  h1 {
    text-align: center;
    font-size: 2rem;
  }
  h2 {
    font-size: 1.1rem;
    margin: 0 0 0.75rem;
  }
  .banner {
    padding: 0.6rem 0.9rem;
    border-radius: 8px;
    margin-bottom: 1rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }
  .banner.err {
    background: rgba(255, 107, 107, 0.15);
    color: #ff8787;
  }
  .banner.warn {
    background: rgba(255, 193, 7, 0.15);
    color: #ffca2c;
  }
  .card {
    border: 1px solid rgba(128, 128, 128, 0.25);
    border-radius: 10px;
    padding: 1rem 1.25rem;
    margin-bottom: 1rem;
  }
  .card.danger {
    border-color: rgba(255, 107, 107, 0.4);
  }
  .rows {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .row {
    display: grid;
    grid-template-columns: auto 1fr auto auto;
    align-items: center;
    gap: 0.75rem;
    padding: 0.35rem 0;
    border-bottom: 1px solid rgba(128, 128, 128, 0.12);
  }
  .dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: #888;
  }
  .dot.on {
    background: #37b24d;
  }
  .name {
    font-weight: 500;
  }
  .meta,
  .key {
    font-size: 0.8rem;
    opacity: 0.65;
  }
  .toggle {
    min-width: 54px;
  }
  .toggle.active {
    border-color: #37b24d;
    color: #37b24d;
  }
  .danger-btn {
    border-color: rgba(255, 107, 107, 0.5);
    color: #ff8787;
  }
</style>
