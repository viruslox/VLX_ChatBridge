let basePath = '';
const mediaQueue = [];
let isPlaying = false;

// Calculate master volume (0.0 to 1.0)
const masterVolume = (window.VLX_CONFIG && typeof window.VLX_CONFIG.VOLUME === 'number')
    ? (window.VLX_CONFIG.VOLUME / 100)
    : 1.0;

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
            }
            else if (data.type === 'sound_command' && data.is_owner_command === true) {
                mediaQueue.push(data);
                processQueue();
            }
        } catch (err) {
            console.error("[Scene Overlay] Parsing error:", err);
        }
    };
}

function processQueue() {
    if (isPlaying || mediaQueue.length === 0) return;

    isPlaying = true;
    const item = mediaQueue.shift();
    const src = `${basePath}/static/chat/${item.filename}`;

    if (item.media_type === 'video') {
        playVideo(src);
    } else {
        playAudio(src);
    }
}

function playAudio(src) {
    console.log("[Playback] Starting AUDIO:", src);
    const audio = document.createElement('audio');
    audio.src = src;
    audio.crossOrigin = "anonymous";
    audio.volume = masterVolume;

    document.body.appendChild(audio);

    const cleanup = () => {
        audio.remove();
        isPlaying = false;
        processQueue();
    };

    audio.play().catch(e => {
        console.warn("[Warning] Audio playback failed:", e);
        cleanup();
    });

    audio.onended = cleanup;
    audio.onerror = () => {
        console.error("[Error] Failed to load audio resource:", src);
        cleanup();
    };
}

function playVideo(src) {
    console.log("[Playback] Starting VIDEO:", src);
    const video = document.createElement('video');
    video.src = src;
    video.crossOrigin = "anonymous";
    video.volume = masterVolume;
    video.autoplay = true;
    video.muted = false;
    video.style.backgroundColor = 'transparent';
    video.style.position = 'absolute';
    video.style.top = '0';
    video.style.left = '0';
    video.style.width = '100vw';
    video.style.height = '100vh';
    video.style.objectFit = 'contain';
    video.style.zIndex = '1000';

    document.body.appendChild(video);

    const cleanup = () => {
        video.remove();
        isPlaying = false;
        processQueue();
    };

    video.play().catch(e => {
        console.warn("[Warning] Video playback failed:", e);
        cleanup();
    });

    video.onended = cleanup;
    video.onerror = () => {
        console.error("[Error] Failed to load video resource:", src);
        cleanup();
    };
}

connect();
