const videoWrapper = document.getElementById('video-wrapper');
const videoElement = document.getElementById('scenes-video');

function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const wsPath = (window.VLX_CONFIG && window.VLX_CONFIG.WEBSOCKET_PATH) || '/websocket';
    
    const socket = new WebSocket(`${protocol}//${host}${wsPath}`);

    socket.onopen = () => console.log("[Scenes Overlay] Connected to ChatBridge.");
    
    socket.onclose = () => {
        console.warn("[Scenes Overlay] Connection lost. Reconnecting in 5s...");
        setTimeout(connect, 5000);
    };

    socket.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            
            // LA MAGIA È QUI: Invece di hardcodare "!sigla", diciamo semplicemente:
            // "Se ChatBridge manda un comando multimediale ed è un VIDEO, riproducilo!"
            if (data.type === 'sound_command' && data.media_type === 'video') {
                playVideo("/static/chat/" + data.filename); 
            }
        } catch (err) {
            console.error("[Scene Overlay] Parsing error:", err);
        }
    };
}

function playVideo(src) {
    if (videoWrapper.style.display === 'block') return;

    videoElement.src = src;
    videoWrapper.style.display = 'block';
    
    setTimeout(() => {
        videoWrapper.style.opacity = '1';
    }, 50);

    videoElement.play().catch(e => {
        console.error("[Scene Overlay] Playback failed:", e);
        closeVideo();
    });

    // Spegnimento automatico quando il video finisce
    videoElement.onended = () => {
        closeVideo();
    };
}

function closeVideo() {
    videoWrapper.style.opacity = '0';
    setTimeout(() => {
        videoWrapper.style.display = 'none';
        videoElement.src = "";
    }, 400);
}

connect();
