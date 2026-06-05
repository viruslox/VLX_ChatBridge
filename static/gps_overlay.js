document.addEventListener("DOMContentLoaded", () => {
    // 1. Selezioniamo gli elementi del DOM
    const boxSpeed = document.getElementById('box-speed');
    const boxPos = document.getElementById('box-pos');
    const boxAlt = document.getElementById('box-alt');

    const valSpeed = document.getElementById('val-speed');
    const valPos = document.getElementById('val-pos');
    const valAlt = document.getElementById('val-alt');

    // 2. Lettura e filtraggio dei Parametri URL (es. ?speed=1&alt=1)
    const params = new URLSearchParams(window.location.search);
    const showSpeed = params.has('speed');
    const showPos = params.has('pos');
    const showAlt = params.has('alt');

    // Fallback: se apri la pagina senza parametri, mostra tutto
    const showAll = (!showSpeed && !showPos && !showAlt);

    const isSpeedVisible = showSpeed || showAll;
    const isPosVisible = showPos || showAll;
    const isAltVisible = showAlt || showAll;

    // Applichiamo il CSS per mostrare i blocchi richiesti
    if (isSpeedVisible) boxSpeed.style.display = 'block';
    if (isPosVisible) boxPos.style.display = 'block';
    if (isAltVisible) boxAlt.style.display = 'block';

    // 3. Connessione al WebSocket di ChatBridge
    function connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const host = window.location.host;
        const wsPath = (window.VLX_CONFIG && window.VLX_CONFIG.WEBSOCKET_PATH) || '/websocket';
        
        const socket = new WebSocket(`${protocol}//${host}${wsPath}`);

        socket.onopen = () => console.log("[VLX GPS Overlay] Connesso a ChatBridge in Real-Time.");
        
        socket.onclose = () => {
            console.warn("[VLX GPS Overlay] Connessione persa. Ritento in 5s...");
            setTimeout(connect, 5000);
        };

        socket.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data);
                
                // Filtriamo solo gli eventi GPS inviati dal server
                if (msg.type === 'gps_update' && msg.data) {
                    const data = msg.data;

                    // Aggiorniamo i valori SOLO se il blocco corrispondente è visibile
                    if (isSpeedVisible && data.speed !== undefined) {
                        const speedKmh = (data.speed * 3.6).toFixed(1); // m/s to km/h
                        valSpeed.innerText = speedKmh;
                    }

                    if (isAltVisible && data.alt !== undefined) {
                        valAlt.innerText = data.alt.toFixed(1);
                    }

                    if (isPosVisible && data.lat !== undefined && data.lon !== undefined) {
                        valPos.innerText = `${data.lat.toFixed(5)}, ${data.lon.toFixed(5)}`;
                    }
                }
            } catch (err) {
                console.error("[VLX GPS Overlay] Errore di parsing del JSON:", err);
            }
        };
    }

    // Avvia la connessione
    connect();
});
