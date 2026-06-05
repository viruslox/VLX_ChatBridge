const gpsContainer = document.getElementById('gps-container');
const speedEl = document.getElementById('gps-speed');
const latEl = document.getElementById('gps-lat');
const lonEl = document.getElementById('gps-lon');
const altEl = document.getElementById('gps-alt');

function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const wsPath = (window.VLX_CONFIG && window.VLX_CONFIG.WEBSOCKET_PATH) || '/ws';

    const socket = new WebSocket(`${protocol}//${host}${wsPath}`);

    socket.onopen = () => console.log("[GPS Overlay] Connected to ChatBridge WebSocket.");

    socket.onclose = () => {
        console.warn("[GPS Overlay] Connection lost. Reconnecting in 5s...");
        setTimeout(connect, 5000);
    };

    socket.onmessage = (event) => {
        try {
            const msg = JSON.parse(event.data);

            if (msg.type === 'gps_update' && msg.data) {
                // Show container on first valid data
                gpsContainer.style.opacity = '1';

                const data = msg.data;

                if (data.speed !== undefined) {
                    speedEl.innerText = `${data.speed} km/h`;
                }
                if (data.lat !== undefined) {
                    latEl.innerText = parseFloat(data.lat).toFixed(4);
                }
                if (data.lon !== undefined) {
                    lonEl.innerText = parseFloat(data.lon).toFixed(4);
                }
                if (data.alt !== undefined) {
                    altEl.innerText = `${data.alt} m`;
                }
            }
        } catch (err) {
            console.error("[GPS Overlay] Parsing error:", err);
        }
    };
}

connect();
