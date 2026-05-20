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

const videoElement = document.getElementById('command-video');
const videoSource = audioCtx.createMediaElementSource(videoElement);
videoSource.connect(compressor);

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
            if (data.type === 'sound_command') {
                mediaQueue.push(data);
                processQueue();
            }
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
    const audio = new Audio(src);
    audio.crossOrigin = "anonymous";
    audio.volume = masterVolume; // Apply Volume

    // Connect to AudioContext for compression
    const source = audioCtx.createMediaElementSource(audio);
    source.connect(compressor);

    audio.play().catch(e => {
        console.warn("[Warning] Audio playback failed:", e);
        source.disconnect();
        isPlaying = false;
        processQueue();
    });

    audio.onended = () => {
        source.disconnect();
        isPlaying = false;
        processQueue();
    };

    audio.onerror = () => {
        console.error("[Error] Failed to load audio resource:", src);
        source.disconnect();
        isPlaying = false;
        processQueue();
    };
}

function playVideo(src) {
    console.log("[Playback] Starting VIDEO:", src);
    videoElement.src = src;
    videoElement.style.display = 'block';
    videoElement.volume = masterVolume; // Apply Volume

    videoElement.play().catch(e => {
        console.warn("[Warning] Video playback failed:", e);
        videoElement.style.display = 'none';
        isPlaying = false;
        processQueue();
    });

    videoElement.onended = () => {
        videoElement.style.display = 'none';
        videoElement.src = "";
        isPlaying = false;
        processQueue();
    };

    videoElement.onerror = () => {
        console.error("[Error] Failed to load video resource:", src);
        videoElement.style.display = 'none';
        isPlaying = false;
        processQueue();
    };
}

connect();
