const videoWrapper = document.getElementById('video-wrapper');
const videoElement = document.getElementById('scenes-video');
let basePath = '';

function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const wsPath = (window.VLX_CONFIG && window.VLX_CONFIG.WEBSOCKET_PATH) || '/websocket';
    basePath = wsPath.substring(0, wsPath.lastIndexOf('/'));
    
    const socket = new WebSocket(`${protocol}//${host}${wsPath}`);

    socket.onopen = () => console.log("[Scenes Overlay] Connected to ChatBridge.");
    
    socket.onclose = () => {
        console.warn("[Scenes Overlay] Connection lost. Reconnecting in 5s...");
        setTimeout(connect, 5000);
    };

    socket.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            
            if (data.type === 'scene_change') {
                console.log("[Scene Overlay] Scene change requested:", data.scene_name);
                // Currently doing nothing visually to the DOM specifically for generic scene names.
                // You can add logic here if you want to switch CSS classes, hide/show things
                // based on data.scene_name (e.g. 'BRB', 'Game', etc.)
            }
            else if (data.type === 'sound_command' && data.media_type === 'video') {
                playVideo(`${basePath}/static/chat/${data.filename}`);
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
