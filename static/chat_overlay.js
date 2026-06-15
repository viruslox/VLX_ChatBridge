// --- Global State Management ---
const mediaQueue = [];
let isPlaying = false;
let basePath = '';

// Calculate master volume (0.0 to 1.0)
const masterVolume = (window.VLX_CONFIG && typeof window.VLX_CONFIG.VOLUME === 'number')
    ? (window.VLX_CONFIG.VOLUME / 100)
    : 1.0;

// --- AudioContext Setup ---
const AudioContext = window.AudioContext || window.webkitAudioContext;
const audioCtx = new AudioContext();
const compressor = audioCtx.createDynamicsCompressor();
compressor.threshold.setValueAtTime(-24, audioCtx.currentTime);
compressor.knee.setValueAtTime(30, audioCtx.currentTime);
compressor.ratio.setValueAtTime(12, audioCtx.currentTime);
compressor.attack.setValueAtTime(0.003, audioCtx.currentTime);
compressor.release.setValueAtTime(0.25, audioCtx.currentTime);
compressor.connect(audioCtx.destination);

function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const wsPath = (window.VLX_CONFIG && window.VLX_CONFIG.WEBSOCKET_PATH) || '/vlxrobot/ws';
    basePath = wsPath.substring(0, wsPath.lastIndexOf('/'));

    const socket = new WebSocket(`${protocol}//${host}${wsPath}`);

    socket.onopen = () => console.log("[System] FX Overlay Connected.");
    socket.onclose = (event) => {
        console.warn(`[System] Connection lost. Reconnecting in 5s...`);
        setTimeout(connect, 5000);
    };
    socket.onerror = (e) => {
        console.error("[Error] WebSocket error:", e);
        socket.close();
    };

    socket.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            if (data.type !== 'sound_command') return;
            if (data.is_owner_command === true) return;

            mediaQueue.push(data);
            processQueue();
        } catch (err) {
            console.error("[Error] Failed to parse incoming message:", err);
        }
    };
}

function processQueue() {
    if (isPlaying || mediaQueue.length === 0) return;

    // Resume AudioContext if suspended
    if (audioCtx.state === 'suspended') {
        audioCtx.resume();
    }

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

    // Connect to AudioContext for compression
    const source = audioCtx.createMediaElementSource(audio);
    source.connect(compressor);

    const cleanup = () => {
        source.disconnect();
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
